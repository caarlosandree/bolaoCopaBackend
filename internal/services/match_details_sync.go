package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"backend/internal/models"
	"backend/internal/repositories"
)

type MatchDetailsSyncService struct {
	DB            *sql.DB
	Matches       *repositories.MatchRepository
	Details       *repositories.MatchDetailsRepository
	TheSportsDB   *TheSportsDBClient
	Logger        *slog.Logger
	LeagueID      string
	Season        string
	LineupWindow  time.Duration
	DailyInterval time.Duration
}

type MatchDetailsSyncSummary struct {
	ScheduleImported int      `json:"schedule_imported"`
	DetailsUpdated   int      `json:"details_updated"`
	Failures         []string `json:"failures"`
}

func NewMatchDetailsSyncService(
	db *sql.DB,
	matches *repositories.MatchRepository,
	details *repositories.MatchDetailsRepository,
	theSportsDB *TheSportsDBClient,
	logger *slog.Logger,
	leagueID string,
	season string,
	lineupWindow time.Duration,
	dailyInterval time.Duration,
) *MatchDetailsSyncService {
	return &MatchDetailsSyncService{
		DB:            db,
		Matches:       matches,
		Details:       details,
		TheSportsDB:   theSportsDB,
		Logger:        logger,
		LeagueID:      leagueID,
		Season:        season,
		LineupWindow:  lineupWindow,
		DailyInterval: dailyInterval,
	}
}

func (s *MatchDetailsSyncService) Start(ctx context.Context) {
	go func() {
		s.runLogged(ctx)
		ticker := time.NewTicker(s.DailyInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runLogged(ctx)
			}
		}
	}()
}

func (s *MatchDetailsSyncService) runLogged(ctx context.Context) {
	summary, err := s.SyncAll(ctx)
	if err != nil {
		s.Logger.Error("sync de detalhes das partidas falhou", "error", err)
		return
	}
	s.Logger.Info(
		"sync de detalhes das partidas concluído",
		"schedule_imported", summary.ScheduleImported,
		"details_updated", summary.DetailsUpdated,
		"failures", len(summary.Failures),
	)
}

func (s *MatchDetailsSyncService) SyncAll(ctx context.Context) (MatchDetailsSyncSummary, error) {
	var summary MatchDetailsSyncSummary

	imported, err := s.SyncSchedule(ctx)
	if err != nil {
		summary.Failures = append(summary.Failures, "thesportsdb schedule: "+err.Error())
	} else {
		summary.ScheduleImported = imported
	}

	updated, failures, err := s.SyncStoredMatchDetails(ctx, false)
	if err != nil {
		summary.Failures = append(summary.Failures, err.Error())
	} else {
		summary.DetailsUpdated = updated
		summary.Failures = append(summary.Failures, failures...)
	}

	if summary.ScheduleImported == 0 && summary.DetailsUpdated == 0 && len(summary.Failures) > 0 {
		return summary, errors.New(strings.Join(summary.Failures, "; "))
	}
	return summary, nil
}

func (s *MatchDetailsSyncService) SyncSchedule(ctx context.Context) (int, error) {
	events, _, err := s.TheSportsDB.ListLeagueSchedule(ctx, s.LeagueID, s.Season)
	if err != nil {
		return 0, err
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	imported := 0
	for i, event := range events {
		matchTime, err := parseTheSportsDBTime(event)
		if err != nil {
			return imported, fmt.Errorf("evento %s: %w", event.IDEvent, err)
		}
		roundNumber := parseRoundNumber(event.Round)
		roundName := roundNameFromNumber(roundNumber)
		groupName := emptyStringToNil(event.Group)
		venue := emptyStringToNil(event.Venue)
		matchNumber := i + 1
		eventID := emptyStringToNil(event.IDEvent)
		homeID := emptyStringToNil(event.HomeTeamID)
		awayID := emptyStringToNil(event.AwayTeamID)
		_, err = s.Matches.UpsertImported(ctx, tx, repositories.ImportedMatch{
			ExternalSource:     theSportsDBSource,
			ExternalID:         event.IDEvent,
			TheSportsDBEventID: eventID,
			TheSportsDBHomeID:  homeID,
			TheSportsDBAwayID:  awayID,
			TournamentName:     "FIFA World Cup 2026",
			RoundNumber:        roundNumber,
			RoundName:          roundName,
			HomeTeam:           event.HomeTeam,
			AwayTeam:           event.AwayTeam,
			MatchTime:          matchTime,
			GroupName:          groupName,
			Venue:              venue,
			MatchNumber:        &matchNumber,
		})
		if err != nil {
			return imported, err
		}
		imported++
	}
	if err := tx.Commit(); err != nil {
		return imported, err
	}
	return imported, nil
}

func (s *MatchDetailsSyncService) SyncStoredMatchDetails(ctx context.Context, force bool) (int, []string, error) {
	matches, err := s.Matches.ListForDetailsSync(ctx)
	if err != nil {
		return 0, nil, err
	}
	updated := 0
	var failures []string
	for _, match := range matches {
		if !force && match.TheSportsDBEventID == nil {
			continue
		}
		if err := s.SyncMatchDetails(ctx, match, force); err != nil {
			failures = append(failures, fmt.Sprintf("match %d: %v", match.ID, err))
			continue
		}
		updated++
	}
	return updated, failures, nil
}

func (s *MatchDetailsSyncService) SyncMatchByID(ctx context.Context, matchID int) error {
	matches, err := s.Matches.ListForDetailsSync(ctx)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if match.ID == matchID {
			return s.SyncMatchDetails(ctx, match, true)
		}
	}
	return sql.ErrNoRows
}

func (s *MatchDetailsSyncService) SyncMatchDetails(ctx context.Context, match repositories.DetailsSyncMatch, force bool) error {
	now := time.Now().UTC()
	statuses := []models.SourceStatus{}
	var media, lineups, stats, events, form json.RawMessage

	if match.TheSportsDBEventID != nil {
		eventRaw, err := s.TheSportsDB.LookupEventRaw(ctx, *match.TheSportsDBEventID)
		statuses = append(statuses, sourceStatus("thesportsdb_v2", "media", err, hasJSON(eventRaw), now))
		if err == nil {
			media = eventRaw
		}

		needsLineups := force
		if !needsLineups {
			hasLineups, _ := s.Details.HasLineups(ctx, match.ID)
			needsLineups = !hasLineups && shouldRefreshLineups(match.MatchTime, now, s.LineupWindow)
		}
		if needsLineups {
			lineupsRaw, err := s.TheSportsDB.LookupLineupRaw(ctx, *match.TheSportsDBEventID)
			statuses = append(statuses, sourceStatus("thesportsdb_v2", "lineups", err, hasRealData(lineupsRaw), now))
			if err == nil && hasRealData(lineupsRaw) {
				lineups = lineupsRaw
			}
		}

		statsRaw, err := s.TheSportsDB.LookupStatsRaw(ctx, *match.TheSportsDBEventID)
		statuses = append(statuses, sourceStatus("thesportsdb_v2", "statistics", err, hasRealData(statsRaw), now))
		if err == nil && hasRealData(statsRaw) {
			stats = statsRaw
		}

		timelineRaw, err := s.TheSportsDB.LookupTimelineRaw(ctx, *match.TheSportsDBEventID)
		statuses = append(statuses, sourceStatus("thesportsdb_v2", "events", err, hasRealData(timelineRaw), now))
		if err == nil && hasRealData(timelineRaw) {
			events = timelineRaw
		}
	}

	formPayload := map[string]json.RawMessage{}
	if match.TheSportsDBHomeID != nil {
		raw, err := s.TheSportsDB.TeamPreviousRaw(ctx, *match.TheSportsDBHomeID)
		statuses = append(statuses, sourceStatus("thesportsdb_v2", "form_home", err, hasJSON(raw), now))
		if err == nil && hasJSON(raw) {
			formPayload["home"] = raw
		}
	}
	if match.TheSportsDBAwayID != nil {
		raw, err := s.TheSportsDB.TeamPreviousRaw(ctx, *match.TheSportsDBAwayID)
		statuses = append(statuses, sourceStatus("thesportsdb_v2", "form_away", err, hasJSON(raw), now))
		if err == nil && hasJSON(raw) {
			formPayload["away"] = raw
		}
	}
	if len(formPayload) > 0 {
		form, _ = json.Marshal(formPayload)
	}

	var lineupSynced *time.Time
	if len(lineups) > 0 {
		lineupSynced = &now
	}
	return s.Details.Upsert(ctx, repositories.MatchDetailsUpsert{
		MatchID:       match.ID,
		RecentForm:    form,
		Lineups:       lineups,
		Statistics:    stats,
		Events:        events,
		Media:         media,
		SourceStatus:  statuses,
		LastSyncedAt:  &now,
		LineupsSynced: lineupSynced,
	})
}

func parseTheSportsDBTime(event TheSportsDBEvent) (time.Time, error) {
	if event.Timestamp != "" {
		if t, err := time.Parse("2006-01-02T15:04:05", event.Timestamp); err == nil {
			return t.UTC(), nil
		}
		if t, err := time.Parse(time.RFC3339, event.Timestamp); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Parse(time.RFC3339, event.Date+"T"+event.Time+"Z")
}

func parseRoundNumber(value string) int {
	n, err := strconvAtoi(value)
	if err != nil || n <= 0 {
		return 900
	}
	return n
}

func roundNameFromNumber(number int) string {
	if number > 0 && number < 100 {
		return fmt.Sprintf("Rodada %d - Fase de Grupos", number)
	}
	switch number {
	case 100:
		return "Round of 32"
	case 101:
		return "Round of 16"
	case 102:
		return "Quarter-finals"
	case 103:
		return "Semi-finals"
	case 104:
		return "Third-place"
	case 105:
		return "Final"
	default:
		return "Copa do Mundo"
	}
}

func strconvAtoi(value string) (int, error) {
	value = strings.TrimSpace(value)
	var n int
	_, err := fmt.Sscanf(value, "%d", &n)
	return n, err
}

func sameTeams(homeA, awayA, homeB, awayB string) bool {
	return normalizeTeam(homeA) == normalizeTeam(homeB) && normalizeTeam(awayA) == normalizeTeam(awayB)
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func shouldRefreshLineups(matchTime, now time.Time, window time.Duration) bool {
	return now.After(matchTime.Add(-window)) && now.Before(matchTime.Add(3*time.Hour))
}

func sourceStatus(source, section string, err error, available bool, syncedAt time.Time) models.SourceStatus {
	status := models.SourceStatus{
		Source:   source,
		Section:  section,
		SyncedAt: &syncedAt,
	}
	if err != nil {
		status.Status = "failed"
		status.Message = err.Error()
		return status
	}
	if !available {
		status.Status = "unavailable"
		return status
	}
	status.Status = "success"
	return status
}

func hasJSON(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null"
}

// isAPIError detecta respostas de erro do TheSportsDB (ex: {"Message": "No data found"}).
// Esses payloads passam em hasJSON mas não contêm dados reais.
func isAPIError(raw json.RawMessage) bool {
	var check struct {
		Message string `json:"Message"`
		Msg     string `json:"message"`
	}
	if err := json.Unmarshal(raw, &check); err != nil {
		return false
	}
	return check.Message != "" || check.Msg != ""
}

func hasRealData(raw json.RawMessage) bool {
	return hasJSON(raw) && !isAPIError(raw)
}
