package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"backend/internal/repositories"
)

const openFootballSource = "openfootball"

type MatchSyncService struct {
	DB              *sql.DB
	Matches         *repositories.MatchRepository
	Score           *MatchScoreService
	HTTPClient      *http.Client
	Logger          *slog.Logger
	OpenFootballURL string
	WorldCup26URL   string
}

type MatchSyncSummary struct {
	Imported       int
	Linked         int
	ScoresUpdated  int
	ScoresSkipped  int
	SourceFailures []string
}

func NewMatchSyncService(db *sql.DB, matches *repositories.MatchRepository, score *MatchScoreService, logger *slog.Logger, openFootballURL, worldCup26BaseURL string) *MatchSyncService {
	return &MatchSyncService{
		DB:              db,
		Matches:         matches,
		Score:           score,
		HTTPClient:      &http.Client{Timeout: 20 * time.Second},
		Logger:          logger,
		OpenFootballURL: openFootballURL,
		WorldCup26URL:   strings.TrimRight(worldCup26BaseURL, "/") + "/get/games",
	}
}

func (s *MatchSyncService) Sync(ctx context.Context) (MatchSyncSummary, error) {
	var summary MatchSyncSummary

	imported, err := s.importOpenFootball(ctx)
	if err != nil {
		summary.SourceFailures = append(summary.SourceFailures, fmt.Sprintf("openfootball: %v", err))
	} else {
		summary.Imported = imported
	}

	liveSummary, err := s.updateFromWorldCup26(ctx)
	if err != nil {
		summary.SourceFailures = append(summary.SourceFailures, fmt.Sprintf("worldcup26: %v", err))
	} else {
		summary.Linked += liveSummary.Linked
		summary.ScoresUpdated += liveSummary.ScoresUpdated
		summary.ScoresSkipped += liveSummary.ScoresSkipped
	}

	if len(summary.SourceFailures) == 2 {
		return summary, fmt.Errorf("todas as fontes de partidas falharam: %s", strings.Join(summary.SourceFailures, "; "))
	}

	return summary, nil
}

func (s *MatchSyncService) SyncSchedule(ctx context.Context) (int, error) {
	return s.importOpenFootball(ctx)
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
		s.runLoggedDueResultSync(ctx, resultCheckAfter)

		ticker := time.NewTicker(retryInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runLoggedDueResultSync(ctx, resultCheckAfter)
			}
		}
	}()
}

func (s *MatchSyncService) runLoggedScheduleImport(ctx context.Context) {
	imported, err := s.importOpenFootball(ctx)
	if err != nil {
		s.Logger.Error("importação de calendário da Copa falhou", "error", err)
		return
	}
	s.Logger.Info("calendário da Copa importado", "imported", imported)
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

type openFootballResponse struct {
	Name    string              `json:"name"`
	Matches []openFootballMatch `json:"matches"`
}

type openFootballMatch struct {
	Round  string `json:"round"`
	Date   string `json:"date"`
	Time   string `json:"time"`
	Team1  string `json:"team1"`
	Team2  string `json:"team2"`
	Group  string `json:"group"`
	Ground string `json:"ground"`
}

func (s *MatchSyncService) importOpenFootball(ctx context.Context) (int, error) {
	var payload openFootballResponse
	if err := s.getJSON(ctx, s.OpenFootballURL, &payload); err != nil {
		return 0, err
	}
	if payload.Name == "" {
		payload.Name = "World Cup 2026"
	}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	imported := 0
	for i, match := range payload.Matches {
		matchTime, err := parseOpenFootballTime(match.Date, match.Time)
		if err != nil {
			return imported, fmt.Errorf("partida %d: %w", i+1, err)
		}

		groupName := emptyStringToNil(match.Group)
		venue := emptyStringToNil(match.Ground)
		matchNumber := i + 1
		_, err = s.Matches.UpsertImported(ctx, tx, repositories.ImportedMatch{
			ExternalSource: openFootballSource,
			ExternalID:     openFootballID(match),
			TournamentName: payload.Name,
			RoundNumber:    roundNumber(match.Round),
			RoundName:      match.Round,
			HomeTeam:       match.Team1,
			AwayTeam:       match.Team2,
			MatchTime:      matchTime,
			GroupName:      groupName,
			Venue:          venue,
			MatchNumber:    &matchNumber,
		})
		if err != nil {
			return imported, err
		}
		imported++
	}

	if err := ensureFirstRoundActive(ctx, tx); err != nil {
		return imported, err
	}

	if err := tx.Commit(); err != nil {
		return imported, err
	}
	return imported, nil
}

func ensureFirstRoundActive(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE rounds
		 SET status = 'active'
		 WHERE id = (
		    SELECT id
		    FROM rounds
		    WHERE status = 'upcoming'
		    ORDER BY number ASC
		    LIMIT 1
		 )
		 AND NOT EXISTS (
		    SELECT 1
		    FROM rounds
		    WHERE status = 'active'
		 )`,
	)
	return err
}

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
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
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
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return err
	}
	return nil
}

var openFootballTimeRE = regexp.MustCompile(`^(\d{2}):(\d{2})\s+UTC([+-]\d{1,2})$`)

func parseOpenFootballTime(dateValue, timeValue string) (time.Time, error) {
	matches := openFootballTimeRE.FindStringSubmatch(strings.TrimSpace(timeValue))
	if len(matches) != 4 {
		return time.Time{}, fmt.Errorf("horário inválido: %q", timeValue)
	}
	hour, err := strconv.Atoi(matches[1])
	if err != nil {
		return time.Time{}, err
	}
	minute, err := strconv.Atoi(matches[2])
	if err != nil {
		return time.Time{}, err
	}
	offsetHour, err := strconv.Atoi(matches[3])
	if err != nil {
		return time.Time{}, err
	}
	date, err := time.Parse("2006-01-02", dateValue)
	if err != nil {
		return time.Time{}, err
	}

	location := time.FixedZone(fmt.Sprintf("UTC%+d", offsetHour), offsetHour*60*60)
	return time.Date(date.Year(), date.Month(), date.Day(), hour, minute, 0, 0, location).UTC(), nil
}

func openFootballID(match openFootballMatch) string {
	parts := []string{match.Date, match.Time, match.Team1, match.Team2, match.Round}
	return normalizeKey(strings.Join(parts, "|"))
}

func roundNumber(roundName string) int {
	normalized := strings.ToLower(strings.TrimSpace(roundName))
	if strings.HasPrefix(normalized, "matchday ") {
		value, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(normalized, "matchday ")))
		if err == nil {
			return value
		}
	}

	switch normalized {
	case "round of 32":
		return 100
	case "round of 16":
		return 101
	case "quarter-finals", "quarterfinals":
		return 102
	case "semi-finals", "semifinals":
		return 103
	case "third-place", "third place", "third-place play-off":
		return 104
	case "final":
		return 105
	default:
		return 900
	}
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
