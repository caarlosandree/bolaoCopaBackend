package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"backend/internal/repositories"
)

const theSportsDBSource = "thesportsdb"

type MatchSyncService struct {
	DB            *sql.DB
	Matches       *repositories.MatchRepository
	Rounds        *repositories.RoundRepository
	Score         *MatchScoreService
	TheSportsDB   *TheSportsDBClient
	LeagueID      string
	Season        string
	HTTPClient    *http.Client
	Logger        *slog.Logger
	WorldCup26URL string
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
	worldCup26BaseURL string,
) *MatchSyncService {
	return &MatchSyncService{
		DB:            db,
		Matches:       matches,
		Rounds:        rounds,
		Score:         score,
		TheSportsDB:   theSportsDB,
		LeagueID:      leagueID,
		Season:        season,
		HTTPClient:    &http.Client{Timeout: 20 * time.Second},
		Logger:        logger,
		WorldCup26URL: strings.TrimRight(worldCup26BaseURL, "/") + "/get/games",
	}
}

func (s *MatchSyncService) SyncSchedule(ctx context.Context) (int, error) {
	return s.importFromTheSportsDB(ctx)
}

func (s *MatchSyncService) SyncResults(ctx context.Context) (MatchSyncSummary, error) {
	result, err := s.updateFromWorldCup26(ctx)
	if err != nil {
		return MatchSyncSummary{}, err
	}
	return MatchSyncSummary{
		Linked:        result.Linked,
		ScoresUpdated: result.ScoresUpdated,
		ScoresSkipped: result.ScoresSkipped,
	}, nil
}

func (s *MatchSyncService) Start(ctx context.Context, retryInterval, resultCheckAfter time.Duration) {
	go func() {
		s.runLoggedScheduleImport(ctx)
		s.runLoggedRoundTransitions(ctx)
		s.runLoggedDueResultSync(ctx, resultCheckAfter)

		ticker := time.NewTicker(retryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runLoggedRoundTransitions(ctx)
				s.runLoggedDueResultSync(ctx, resultCheckAfter)
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
	summary, err := s.updateDueResultsFromWorldCup26(ctx, resultCheckAfter)
	if err != nil {
		s.Logger.Error("sync de resultados da Copa falhou", "error", err)
		return
	}
	if !summary.Checked {
		return
	}
	s.Logger.Info(
		"sync de resultados da Copa concluído",
		"linked", summary.Linked,
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
	// Rounds de mata-mata vêm com números maiores — mapeia para constantes internas
	// TheSportsDB tende a usar rounds 4+ para eliminatórias
	switch n {
	case 4:
		return 100 // Fase de 32 (Copa 2026)
	case 5:
		return 101 // Oitavas
	case 6:
		return 102 // Quartas
	case 7:
		return 103 // Semi
	case 8:
		return 104 // 3º lugar
	case 9:
		return 105 // Final
	default:
		return n + 90 // fallback para rounds inesperados
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

// ==================== WorldCup26 (sync de resultados) ====================

type worldCup26Response struct {
	Games []worldCup26Game `json:"games"`
}

type worldCup26Game struct {
	ID             string `json:"id"`
	HomeScore      string `json:"home_score"`
	AwayScore      string `json:"away_score"`
	Group          string `json:"group"`
	Matchday       string `json:"matchday"`
	Finished       string `json:"finished"`
	TimeElapsed    string `json:"time_elapsed"`
	Type           string `json:"type"`
	HomeTeamNameEN string `json:"home_team_name_en"`
	AwayTeamNameEN string `json:"away_team_name_en"`
}

type worldCup26Summary struct {
	Checked       bool
	Linked        int
	ScoresUpdated int
	ScoresSkipped int
}

func (s *MatchSyncService) updateDueResultsFromWorldCup26(ctx context.Context, resultCheckAfter time.Duration) (worldCup26Summary, error) {
	var summary worldCup26Summary
	cutoff := time.Now().UTC().Add(-resultCheckAfter)
	hasDue, err := s.Matches.HasMatchesDueForResultCheck(ctx, cutoff)
	if err != nil {
		return summary, err
	}
	if !hasDue {
		return summary, nil
	}

	summary, err = s.updateFromWorldCup26(ctx)
	summary.Checked = true
	return summary, err
}

func (s *MatchSyncService) updateFromWorldCup26(ctx context.Context) (worldCup26Summary, error) {
	var summary worldCup26Summary
	var payload worldCup26Response
	if err := s.getJSON(ctx, s.WorldCup26URL, &payload); err != nil {
		return summary, err
	}

	matches, err := s.Matches.ListForSync(ctx)
	if err != nil {
		return summary, err
	}

	byWorldCup26ID := make(map[string]repositories.MatchSyncRow)
	for _, match := range matches {
		if match.WorldCup26MatchID != nil {
			byWorldCup26ID[*match.WorldCup26MatchID] = match
		}
	}

	for _, game := range payload.Games {
		match, ok := byWorldCup26ID[game.ID]
		if !ok {
			var found bool
			match, found = findWorldCup26Match(matches, game)
			if !found {
				summary.ScoresSkipped++
				continue
			}
			if match.WorldCup26MatchID == nil {
				if err := s.linkWorldCup26Match(ctx, match.ID, game.ID); err != nil {
					return summary, err
				}
				summary.Linked++
				id := game.ID
				match.WorldCup26MatchID = &id
				byWorldCup26ID[game.ID] = match
			}
		}

		if !isWorldCup26Finished(game) {
			continue
		}

		homeScore, err := strconv.Atoi(strings.TrimSpace(game.HomeScore))
		if err != nil {
			summary.ScoresSkipped++
			continue
		}
		awayScore, err := strconv.Atoi(strings.TrimSpace(game.AwayScore))
		if err != nil {
			summary.ScoresSkipped++
			continue
		}

		result, err := s.Score.UpdateFinalScore(ctx, match.ID, homeScore, awayScore)
		if err != nil {
			return summary, err
		}
		if result.Changed {
			summary.ScoresUpdated++
		}
	}

	return summary, nil
}

func (s *MatchSyncService) linkWorldCup26Match(ctx context.Context, matchID int, worldCup26MatchID string) error {
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := s.Matches.LinkWorldCup26Match(ctx, tx, matchID, worldCup26MatchID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *MatchSyncService) getJSON(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "bolao-copa/1.0")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s retornou HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

func findWorldCup26Match(matches []repositories.MatchSyncRow, game worldCup26Game) (repositories.MatchSyncRow, bool) {
	home := normalizeTeam(game.HomeTeamNameEN)
	away := normalizeTeam(game.AwayTeamNameEN)
	group := normalizeGroup(game.Group)

	for _, match := range matches {
		if normalizeTeam(match.HomeTeam) != home || normalizeTeam(match.AwayTeam) != away {
			continue
		}
		if group == "" {
			return match, true
		}
		if match.GroupName != nil && normalizeGroup(*match.GroupName) == group {
			return match, true
		}
	}
	return repositories.MatchSyncRow{}, false
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

func isWorldCup26Finished(game worldCup26Game) bool {
	return strings.EqualFold(game.Finished, "true") ||
		strings.EqualFold(game.Finished, "finished") ||
		strings.EqualFold(game.TimeElapsed, "finished")
}
