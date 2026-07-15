package services

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"backend/internal/repositories"
)

const theSportsDBSource = "thesportsdb"

type MatchSyncService struct {
	DB          *sql.DB
	Matches     *repositories.MatchRepository
	Rounds      *repositories.RoundRepository
	Score       *MatchScoreService
	TheSportsDB *TheSportsDBClient
	LeagueID    string
	Season      string
	Logger      *slog.Logger
}

type MatchSyncSummary struct {
	Imported       int
	Linked         int
	ScoresUpdated  int
	ScoresSkipped  int
	SourceFailures []string
}

func NewMatchSyncService(
	db *sql.DB,
	matches *repositories.MatchRepository,
	rounds *repositories.RoundRepository,
	score *MatchScoreService,
	theSportsDB *TheSportsDBClient,
	leagueID, season string,
	logger *slog.Logger,
) *MatchSyncService {
	return &MatchSyncService{
		DB:          db,
		Matches:     matches,
		Rounds:      rounds,
		Score:       score,
		TheSportsDB: theSportsDB,
		LeagueID:    leagueID,
		Season:      season,
		Logger:      logger,
	}
}

func (s *MatchSyncService) SyncSchedule(ctx context.Context) (int, error) {
	return s.importFromTheSportsDB(ctx)
}

func (s *MatchSyncService) SyncResults(ctx context.Context) (MatchSyncSummary, error) {
	result, err := s.updateResultsFromTheSportsDB(ctx)
	if err != nil {
		return MatchSyncSummary{}, err
	}
	return MatchSyncSummary{
		ScoresUpdated: result.ScoresUpdated,
		ScoresSkipped: result.ScoresSkipped,
	}, nil
}

func (s *MatchSyncService) Start(ctx context.Context, retryInterval, resultCheckAfter time.Duration) {
	go func() {
		s.runLoggedScheduleImport(ctx)
		// Resultados primeiro, depois transição de rodadas — assim uma rodada
		// cuja última partida acabou de virar "finished" fecha no mesmo ciclo.
		s.runLoggedDueResultSync(ctx, resultCheckAfter)
		s.runLoggedRoundTransitions(ctx)

		ticker := time.NewTicker(retryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runLoggedDueResultSync(ctx, resultCheckAfter)
				s.runLoggedRoundTransitions(ctx)
			}
		}
	}()
}

func (s *MatchSyncService) runLoggedScheduleImport(ctx context.Context) {
	imported, err := s.importFromTheSportsDB(ctx)
	if err != nil {
		s.Logger.Error("importação de calendário da Copa falhou", "error", err)
		return
	}
	s.Logger.Info("calendário da Copa importado", "imported", imported)
}

func (s *MatchSyncService) runLoggedRoundTransitions(ctx context.Context) {
	activated, err := s.Rounds.ActivateDueRounds(ctx)
	if err != nil {
		s.Logger.Error("falha ao ativar rodadas pendentes", "error", err)
	} else if activated > 0 {
		s.Logger.Info("rodadas ativadas automaticamente", "count", activated)
	}

	finished, err := s.Rounds.FinishCompletedRounds(ctx)
	if err != nil {
		s.Logger.Error("falha ao finalizar rodadas concluídas", "error", err)
	} else if finished > 0 {
		s.Logger.Info("rodadas finalizadas automaticamente", "count", finished)
	}
}

func (s *MatchSyncService) runLoggedDueResultSync(ctx context.Context, resultCheckAfter time.Duration) {
	summary, err := s.updateDueResultsFromTheSportsDB(ctx, resultCheckAfter)
	if err != nil {
		s.Logger.Error("sync de resultados da Copa falhou", "error", err)
		return
	}
	if !summary.Checked {
		return
	}
	s.Logger.Info(
		"sync de resultados da Copa concluído",
		"scores_updated", summary.ScoresUpdated,
		"scores_skipped", summary.ScoresSkipped,
	)
}

// importFromTheSportsDB importa o calendário da Copa via TheSportsDB v2.
// Cada intRound agrupa todos os jogos daquela rodada (ex: Round 1 = 24 jogos da fase de grupos).
func (s *MatchSyncService) importFromTheSportsDB(ctx context.Context) (int, error) {
	if s.TheSportsDB == nil {
		return 0, fmt.Errorf("TheSportsDB client não configurado")
	}

	events, _, err := s.TheSportsDB.ListLeagueSchedule(ctx, s.LeagueID, s.Season)
	if err != nil {
		return 0, fmt.Errorf("falha ao buscar calendário do TheSportsDB: %w", err)
	}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	imported := 0
	for i, event := range events {
		matchTime, err := parseTheSportsDBTimestamp(event.Timestamp)
		if err != nil {
			return imported, fmt.Errorf("evento %d (%s): timestamp inválido %q: %w", i+1, event.IDEvent, event.Timestamp, err)
		}

		roundNum := parseIntRound(event.Round)
		roundName := theSportsDBRoundName(roundNum)
		eventID := event.IDEvent
		homeTeamID := event.HomeTeamID
		awayTeamID := event.AwayTeamID
		groupName := emptyStringToNil(event.Group)
		venue := emptyStringToNil(event.Venue)
		matchNum := i + 1
		isKnockout := roundNum >= 100

		_, err = s.Matches.UpsertImported(ctx, tx, repositories.ImportedMatch{
			ExternalSource:     theSportsDBSource,
			ExternalID:         eventID,
			TournamentName:     "FIFA World Cup 2026",
			RoundNumber:        roundNum,
			RoundName:          roundName,
			HomeTeam:           event.HomeTeam,
			AwayTeam:           event.AwayTeam,
			MatchTime:          matchTime,
			GroupName:          groupName,
			Venue:              venue,
			MatchNumber:        &matchNum,
			TheSportsDBEventID: &eventID,
			TheSportsDBHomeID:  &homeTeamID,
			TheSportsDBAwayID:  &awayTeamID,
			IsKnockout:         isKnockout,
		})
		if err != nil {
			return imported, fmt.Errorf("evento %s: %w", eventID, err)
		}
		imported++
	}

	if err := tx.Commit(); err != nil {
		return imported, err
	}
	return imported, nil
}

// theSportsDBRoundName mapeia o número de round para o nome exibido no bolão.
func theSportsDBRoundName(roundNum int) string {
	switch roundNum {
	case 1:
		return "Fase de Grupos — Rodada 1"
	case 2:
		return "Fase de Grupos — Rodada 2"
	case 3:
		return "Fase de Grupos — Rodada 3"
	case 100:
		return "Fase de 32"
	case 101:
		return "Oitavas de Final"
	case 102:
		return "Quartas de Final"
	case 103:
		return "Semifinal"
	case 104:
		return "Disputa de 3º Lugar"
	case 105:
		return "Final"
	default:
		return fmt.Sprintf("Rodada %d", roundNum)
	}
}

// parseIntRound converte a string intRound do TheSportsDB para o número interno de rodada.
func parseIntRound(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 900
	}
	// Fase de grupos: rounds 1, 2, 3
	if n <= 3 {
		return n
	}
	// Rounds de mata-mata podem vir como tamanho da fase no TheSportsDB.
	// Na Copa 2026 a API também usa 125 (quartas) e 150 (semis).
	switch n {
	case 32:
		return 100 // Fase de 32
	case 16:
		return 101 // Oitavas
	case 125:
		return 102 // Quartas (TheSportsDB WC 2026)
	case 150:
		return 103 // Semifinal (TheSportsDB WC 2026)
	case 160, 170:
		return 104 // 3º lugar (códigos previstos)
	case 175, 180, 200:
		return 105 // Final (códigos previstos)
	case 4:
		return 100 // Compatibilidade com payloads antigos
	case 5:
		return 101 // Compatibilidade com payloads antigos
	case 6:
		return 102 // Compatibilidade com payloads antigos
	case 7:
		return 103 // Compatibilidade com payloads antigos
	case 8:
		return 104 // Compatibilidade com payloads antigos
	case 9:
		return 105 // Compatibilidade com payloads antigos
	default:
		return n
	}
}

func parseTheSportsDBTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("timestamp vazio")
	}
	// TheSportsDB v2 retorna timestamps UTC sem sufixo Z: "2026-06-11T19:00:00"
	t, err := time.Parse("2006-01-02T15:04:05", raw)
	if err != nil {
		// Tenta com Z caso venha com sufixo
		t, err = time.Parse(time.RFC3339, raw)
		if err != nil {
			return time.Time{}, err
		}
	}
	return t.UTC(), nil
}

// ==================== TheSportsDB (sync de resultados) ====================

type theSportsDBResultSummary struct {
	Checked       bool
	ScoresUpdated int
	ScoresSkipped int
}

func theSportsDBStatusToInternal(status string) string {
	// TheSportsDB usa, entre outros:
	// FT = full time, AET = after extra time, AP = after penalties.
	// AET/AP eram mapeados para "scheduled" (default) e nunca finalizavam a partida.
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "ft", "aet", "ap", "pen", "aft", "finished", "match finished":
		return "finished"
	case "1h", "ht", "2h", "et", "p", "live", "susp", "in play", "inplay":
		return "ongoing"
	case "pst", "can", "postponed", "cancelled", "canceled", "abd":
		return "scheduled"
	default:
		return "scheduled"
	}
}

// theSportsDBAdvanceMethod deriva o método de desempate (et vs penalties) do status do TheSportsDB.
// Retorna "et" para prorrogação, "penalties" para pênaltis, ou "" se não se aplica.
func theSportsDBAdvanceMethod(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "et", "aet":
		return "et"
	case "p", "ap", "pen", "penalties":
		return "penalties"
	default:
		return ""
	}
}

// deriveKnockoutWinner devolve "home"/"away" a partir do placar e, se empate,
// dos gols/pênaltis extras (intHomeScoreExtra/intAwayScoreExtra).
func deriveKnockoutWinner(homeScore, awayScore int, homeExtra, awayExtra *int) *string {
	if homeScore > awayScore {
		w := "home"
		return &w
	}
	if homeScore < awayScore {
		w := "away"
		return &w
	}
	if homeExtra != nil && awayExtra != nil {
		if *homeExtra > *awayExtra {
			w := "home"
			return &w
		}
		if *homeExtra < *awayExtra {
			w := "away"
			return &w
		}
	}
	return nil
}

func ptrEqualString(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func parseTheSportsDBScore(s *string) (int, bool) {
	if s == nil {
		return 0, false
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (s *MatchSyncService) updateDueResultsFromTheSportsDB(ctx context.Context, resultCheckAfter time.Duration) (theSportsDBResultSummary, error) {
	var summary theSportsDBResultSummary
	cutoff := time.Now().UTC().Add(-resultCheckAfter)
	hasDue, err := s.Matches.HasMatchesDueForResultCheck(ctx, cutoff)
	if err != nil {
		return summary, err
	}
	if !hasDue {
		return summary, nil
	}
	summary, err = s.updateResultsFromTheSportsDB(ctx)
	summary.Checked = true
	return summary, err
}

func (s *MatchSyncService) updateResultsFromTheSportsDB(ctx context.Context) (theSportsDBResultSummary, error) {
	var summary theSportsDBResultSummary
	events, _, err := s.TheSportsDB.ListLeagueSchedule(ctx, s.LeagueID, s.Season)
	if err != nil {
		return summary, fmt.Errorf("falha ao buscar schedule do TheSportsDB: %w", err)
	}

	matches, err := s.Matches.ListForSync(ctx)
	if err != nil {
		return summary, err
	}

	byEventID := make(map[string]repositories.MatchSyncRow)
	for _, match := range matches {
		if match.TheSportsDBEventID != nil {
			byEventID[*match.TheSportsDBEventID] = match
		}
	}

	for _, event := range events {
		match, ok := byEventID[event.IDEvent]
		if !ok {
			match, ok = s.findMatchByTeamsAndTime(matches, event)
			if !ok {
				summary.ScoresSkipped++
				continue
			}
		}

		newStatus := theSportsDBStatusToInternal(event.Status)
		homeScore, homeOK := parseTheSportsDBScore(event.HomeScore)
		awayScore, awayOK := parseTheSportsDBScore(event.AwayScore)

		// Jogo finalizado: usa UpdateFinalScore (recalcula pontos dos palpites)
		if newStatus == "finished" && homeOK && awayOK {
			scoreAlreadyFinal := match.Status == "finished" &&
				match.HomeScore != nil && match.AwayScore != nil &&
				*match.HomeScore == homeScore && *match.AwayScore == awayScore

			if !scoreAlreadyFinal {
				_, err := s.Score.UpdateFinalScore(ctx, match.ID, homeScore, awayScore)
				if err != nil {
					return summary, err
				}
				summary.ScoresUpdated++
			}

			// Mata-mata: winner_team + advance_method (AET/AP e placares com vencedor)
			if match.IsKnockout {
				changed, err := s.syncKnockoutResult(ctx, match, event, homeScore, awayScore)
				if err != nil {
					return summary, err
				}
				if changed && scoreAlreadyFinal {
					summary.ScoresUpdated++
				}
			} else if scoreAlreadyFinal {
				continue
			}
			continue
		}

		// Jogo em andamento ou scheduled: atualiza score/status direto se houver mudança
		if match.Status == newStatus {
			if !homeOK || !awayOK {
				continue
			}
			if match.HomeScore != nil && match.AwayScore != nil &&
				*match.HomeScore == homeScore && *match.AwayScore == awayScore {
				continue
			}
		}

		if err := s.updateMatchStatusAndScore(ctx, match.ID, newStatus, homeOK, awayOK, homeScore, awayScore); err != nil {
			return summary, err
		}
		summary.ScoresUpdated++
	}

	return summary, nil
}

func (s *MatchSyncService) findMatchByTeamsAndTime(matches []repositories.MatchSyncRow, event TheSportsDBEvent) (repositories.MatchSyncRow, bool) {
	home := resolveTeamAlias(normalizeTeam(event.HomeTeam))
	away := resolveTeamAlias(normalizeTeam(event.AwayTeam))
	eventTime, _ := parseTheSportsDBTimestamp(event.Timestamp)

	for _, match := range matches {
		if resolveTeamAlias(normalizeTeam(match.HomeTeam)) != home ||
			resolveTeamAlias(normalizeTeam(match.AwayTeam)) != away {
			continue
		}
		if eventTime.IsZero() || match.MatchTime.Equal(eventTime) || match.MatchTime.Equal(eventTime.Add(24*time.Hour)) {
			return match, true
		}
	}
	return repositories.MatchSyncRow{}, false
}

func (s *MatchSyncService) updateMatchStatusAndScore(ctx context.Context, matchID int, status string, homeOK, awayOK bool, homeScore, awayScore int) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if homeOK && awayOK {
		_, err = tx.ExecContext(ctx,
			`UPDATE matches SET status = $1, home_score = $2, away_score = $3, updated_at = CURRENT_TIMESTAMP WHERE id = $4`,
			status, homeScore, awayScore, matchID,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`UPDATE matches SET status = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
			status, matchID,
		)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// syncKnockoutResult preenche winner_team e advance_method em jogos de mata-mata
// finalizados via AET/AP (ou com placar decisivo). Recalcula bônus de palpites.
// Retorna true se houve alteração persistida.
func (s *MatchSyncService) syncKnockoutResult(
	ctx context.Context,
	match repositories.MatchSyncRow,
	event TheSportsDBEvent,
	homeScore, awayScore int,
) (bool, error) {
	advanceMethod := theSportsDBAdvanceMethod(event.Status)
	homeExtra, homeExtraOK := parseTheSportsDBScore(event.HomeScoreExtra)
	awayExtra, awayExtraOK := parseTheSportsDBScore(event.AwayScoreExtra)

	var homeExtraPtr, awayExtraPtr *int
	if homeExtraOK {
		homeExtraPtr = &homeExtra
	}
	if awayExtraOK {
		awayExtraPtr = &awayExtra
	}

	winnerTeam := deriveKnockoutWinner(homeScore, awayScore, homeExtraPtr, awayExtraPtr)

	// Schedule v2 muitas vezes omite extras/result em AP. Busca o lookup completo.
	if winnerTeam == nil && strings.EqualFold(strings.TrimSpace(event.Status), "AP") &&
		s.TheSportsDB != nil && event.IDEvent != "" {
		if detailed, err := s.TheSportsDB.LookupEvent(ctx, event.IDEvent); err == nil && detailed != nil {
			if advanceMethod == "" {
				advanceMethod = theSportsDBAdvanceMethod(detailed.Status)
			}
			hEx, hOK := parseTheSportsDBScore(detailed.HomeScoreExtra)
			aEx, aOK := parseTheSportsDBScore(detailed.AwayScoreExtra)
			if hOK {
				homeExtraPtr = &hEx
			}
			if aOK {
				awayExtraPtr = &aEx
			}
			winnerTeam = deriveKnockoutWinner(homeScore, awayScore, homeExtraPtr, awayExtraPtr)
		} else if err != nil {
			s.Logger.Warn("lookup de evento para desempate falhou",
				"event_id", event.IDEvent, "match_id", match.ID, "error", err)
		}
	}

	// AET com placar decisivo e sem status de método explícito → prorrogação
	if advanceMethod == "" && homeScore != awayScore &&
		strings.EqualFold(strings.TrimSpace(event.Status), "AET") {
		advanceMethod = "et"
	}

	// Nada a persistir
	if winnerTeam == nil && advanceMethod == "" {
		return false, nil
	}

	var advancePtr *string
	if advanceMethod != "" {
		advancePtr = &advanceMethod
	}

	if ptrEqualString(match.WinnerTeam, winnerTeam) &&
		ptrEqualString(match.AdvanceMethod, advancePtr) {
		return false, nil
	}

	// Sem vencedor mas com método: grava só o método (admin pode completar).
	// Com vencedor: UpdateKnockoutResult recalcula bônus.
	if winnerTeam == nil {
		_, err := s.DB.ExecContext(ctx,
			`UPDATE matches SET advance_method = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
			advanceMethod, match.ID,
		)
		return err == nil, err
	}

	_, err := s.Score.UpdateKnockoutResult(ctx, match.ID, winnerTeam, advancePtr)
	return err == nil, err
}

// resolveTeamAlias normaliza variantes de nome de seleção para uma forma canônica.
func resolveTeamAlias(normalized string) string {
	switch normalized {
	case "korearepublic", "republicofkorea":
		return "southkorea"
	case "czechia":
		return "czechrepublic"
	case "trinidadtobago":
		return "trinidadandtobago"
	case "ivorycoast", "cotedivoire":
		return "ivorycoast"
	case "unitedstatesofamerica", "unitedstates":
		return "usa"
	case "democraticrepublicofthecongo", "drcongo":
		return "congo"
	}
	return normalized
}

func normalizeTeam(value string) string {
	value = strings.ReplaceAll(value, "&", "and")
	return normalizeKey(value)
}

func normalizeGroup(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.TrimPrefix(strings.ToLower(value), "group ")
	return normalizeKey(value)
}

func normalizeKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func emptyStringToNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
