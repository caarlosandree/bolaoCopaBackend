package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"backend/internal/models"
)

type MatchDetailsRepository struct {
	DB *sql.DB
}

func NewMatchDetailsRepository(db *sql.DB) *MatchDetailsRepository {
	return &MatchDetailsRepository{DB: db}
}

type MatchDetailsUpsert struct {
	MatchID       int
	Odds          json.RawMessage
	Predictions   json.RawMessage
	RecentForm    json.RawMessage
	HeadToHead    json.RawMessage
	Lineups       json.RawMessage
	Statistics    json.RawMessage
	Injuries      json.RawMessage
	Events        json.RawMessage
	Media         json.RawMessage
	SourceStatus  []models.SourceStatus
	LastSyncedAt  *time.Time
	LineupsSynced *time.Time
}

func (r *MatchDetailsRepository) FindByMatchID(ctx context.Context, matchID int) (*models.MatchDetails, error) {
	row := r.DB.QueryRowContext(ctx,
		`SELECT match_id, odds, predictions, recent_form, head_to_head, lineups,
		        statistics, injuries, events, media, source_status, last_synced_at,
		        lineups_synced_at, updated_at
		 FROM match_details
		 WHERE match_id = $1`,
		matchID,
	)
	return scanMatchDetails(row)
}

func (r *MatchDetailsRepository) Upsert(ctx context.Context, d MatchDetailsUpsert) error {
	status, err := json.Marshal(d.SourceStatus)
	if err != nil {
		return err
	}
	_, err = r.DB.ExecContext(ctx,
		`INSERT INTO match_details (
		    match_id, odds, predictions, recent_form, head_to_head, lineups,
		    statistics, injuries, events, media, source_status, last_synced_at,
		    lineups_synced_at, updated_at
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CURRENT_TIMESTAMP)
		 ON CONFLICT (match_id)
		 DO UPDATE SET
		    odds = COALESCE(EXCLUDED.odds, match_details.odds),
		    predictions = COALESCE(EXCLUDED.predictions, match_details.predictions),
		    recent_form = COALESCE(EXCLUDED.recent_form, match_details.recent_form),
		    head_to_head = COALESCE(EXCLUDED.head_to_head, match_details.head_to_head),
		    lineups = COALESCE(EXCLUDED.lineups, match_details.lineups),
		    statistics = COALESCE(EXCLUDED.statistics, match_details.statistics),
		    injuries = COALESCE(EXCLUDED.injuries, match_details.injuries),
		    events = COALESCE(EXCLUDED.events, match_details.events),
		    media = COALESCE(EXCLUDED.media, match_details.media),
		    source_status = EXCLUDED.source_status,
		    last_synced_at = COALESCE(EXCLUDED.last_synced_at, match_details.last_synced_at),
		    lineups_synced_at = COALESCE(EXCLUDED.lineups_synced_at, match_details.lineups_synced_at),
		    updated_at = CURRENT_TIMESTAMP`,
		d.MatchID,
		nullJSON(d.Odds),
		nullJSON(d.Predictions),
		nullJSON(d.RecentForm),
		nullJSON(d.HeadToHead),
		nullJSON(d.Lineups),
		nullJSON(d.Statistics),
		nullJSON(d.Injuries),
		nullJSON(d.Events),
		nullJSON(d.Media),
		status,
		d.LastSyncedAt,
		d.LineupsSynced,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMatchDetails(row rowScanner) (*models.MatchDetails, error) {
	var d models.MatchDetails
	var odds, predictions, form, h2h, lineups, stats, injuries, events, media, sourceStatus sql.NullString
	err := row.Scan(
		&d.MatchID,
		&odds,
		&predictions,
		&form,
		&h2h,
		&lineups,
		&stats,
		&injuries,
		&events,
		&media,
		&sourceStatus,
		&d.LastSyncedAt,
		&d.LineupsSynced,
		&d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	d.Odds = rawIfValid(odds)
	d.Predictions = rawIfValid(predictions)
	d.RecentForm = rawIfValid(form)
	d.HeadToHead = rawIfValid(h2h)
	d.Lineups = rawIfValid(lineups)
	d.Statistics = rawIfValid(stats)
	d.Injuries = rawIfValid(injuries)
	d.Events = rawIfValid(events)
	d.Media = rawIfValid(media)
	if sourceStatus.Valid {
		_ = json.Unmarshal([]byte(sourceStatus.String), &d.SourceStatus)
	}
	if d.SourceStatus == nil {
		d.SourceStatus = []models.SourceStatus{}
	}
	return &d, nil
}

func rawIfValid(value sql.NullString) json.RawMessage {
	if value.Valid && value.String != "" {
		return json.RawMessage(value.String)
	}
	return nil
}

func nullJSON(value json.RawMessage) any {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	return value
}
