package repositories

import (
	"context"
	"database/sql"
	"time"

	"backend/internal/models"
)

type MatchRepository struct {
	DB *sql.DB
}

func NewMatchRepository(db *sql.DB) *MatchRepository {
	return &MatchRepository{DB: db}
}

type MatchTime struct {
	MatchTime time.Time
	Status    string
}

type ImportedMatch struct {
	ExternalSource    string
	ExternalID        string
	WorldCup26MatchID *string
	TournamentName    string
	RoundNumber       int
	RoundName         string
	HomeTeam          string
	AwayTeam          string
	MatchTime         time.Time
	GroupName         *string
	Venue             *string
	MatchNumber       *int
}

type MatchSyncRow struct {
	ID                int
	HomeTeam          string
	AwayTeam          string
	HomeScore         *int
	AwayScore         *int
	Status            string
	MatchTime         time.Time
	GroupName         *string
	WorldCup26MatchID *string
}

func (r *MatchRepository) FindMatchTime(ctx context.Context, matchID int) (*MatchTime, error) {
	var m MatchTime
	err := r.DB.QueryRowContext(ctx,
		`SELECT match_time, status FROM matches WHERE id = $1`,
		matchID,
	).Scan(&m.MatchTime, &m.Status)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *MatchRepository) UpdateScoreAndStatus(ctx context.Context, tx *sql.Tx, matchID, homeScore, awayScore int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE matches
		 SET home_score = $1, away_score = $2, status = 'finished', updated_at = CURRENT_TIMESTAMP
		 WHERE id = $3`,
		homeScore, awayScore, matchID,
	)
	return err
}

func (r *MatchRepository) UpsertImported(ctx context.Context, tx *sql.Tx, m ImportedMatch) (int, error) {
	var tournamentID int
	err := tx.QueryRowContext(ctx,
		`INSERT INTO tournaments (name, active)
		 VALUES ($1, TRUE)
		 ON CONFLICT DO NOTHING
		 RETURNING id`,
		m.TournamentName,
	).Scan(&tournamentID)
	if err != nil {
		if err != sql.ErrNoRows {
			return 0, err
		}
		err = tx.QueryRowContext(ctx,
			`SELECT id FROM tournaments WHERE name = $1 ORDER BY id LIMIT 1`,
			m.TournamentName,
		).Scan(&tournamentID)
		if err != nil {
			return 0, err
		}
	}

	var roundID int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO rounds (tournament_id, number, name, status)
		 VALUES ($1, $2, $3, 'upcoming')
		 ON CONFLICT (tournament_id, number)
		 DO UPDATE SET name = EXCLUDED.name
		 RETURNING id`,
		tournamentID, m.RoundNumber, m.RoundName,
	).Scan(&roundID)
	if err != nil {
		return 0, err
	}

	var matchID int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO matches (
		    round_id, home_team, away_team, match_time, status,
		    external_source, external_id, worldcup26_match_id, group_name, venue, match_number
		 )
		 VALUES ($1, $2, $3, $4, 'scheduled', $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (external_source, external_id)
		 DO UPDATE SET
		    round_id = EXCLUDED.round_id,
		    home_team = EXCLUDED.home_team,
		    away_team = EXCLUDED.away_team,
		    match_time = EXCLUDED.match_time,
		    worldcup26_match_id = COALESCE(matches.worldcup26_match_id, EXCLUDED.worldcup26_match_id),
		    group_name = EXCLUDED.group_name,
		    venue = EXCLUDED.venue,
		    match_number = EXCLUDED.match_number,
		    updated_at = CURRENT_TIMESTAMP
		 RETURNING id`,
		roundID,
		m.HomeTeam,
		m.AwayTeam,
		m.MatchTime,
		m.ExternalSource,
		m.ExternalID,
		m.WorldCup26MatchID,
		m.GroupName,
		m.Venue,
		m.MatchNumber,
	).Scan(&matchID)
	if err != nil {
		return 0, err
	}
	return matchID, nil
}

func (r *MatchRepository) ListForSync(ctx context.Context) ([]MatchSyncRow, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, home_team, away_team, home_score, away_score, status, match_time, group_name, worldcup26_match_id
		 FROM matches
		 ORDER BY match_time ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []MatchSyncRow
	for rows.Next() {
		var m MatchSyncRow
		var homeScore, awayScore sql.NullInt32
		var groupName, worldCup26MatchID sql.NullString
		if err := rows.Scan(
			&m.ID,
			&m.HomeTeam,
			&m.AwayTeam,
			&homeScore,
			&awayScore,
			&m.Status,
			&m.MatchTime,
			&groupName,
			&worldCup26MatchID,
		); err != nil {
			return nil, err
		}
		if homeScore.Valid {
			score := int(homeScore.Int32)
			m.HomeScore = &score
		}
		if awayScore.Valid {
			score := int(awayScore.Int32)
			m.AwayScore = &score
		}
		if groupName.Valid {
			value := groupName.String
			m.GroupName = &value
		}
		if worldCup26MatchID.Valid {
			value := worldCup26MatchID.String
			m.WorldCup26MatchID = &value
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func (r *MatchRepository) HasMatchesDueForResultCheck(ctx context.Context, cutoff time.Time) (bool, error) {
	var hasDue bool
	err := r.DB.QueryRowContext(ctx,
		`SELECT EXISTS (
		    SELECT 1
		    FROM matches
		    WHERE status <> 'finished'
		      AND match_time <= $1
		)`,
		cutoff,
	).Scan(&hasDue)
	return hasDue, err
}

func (r *MatchRepository) LinkWorldCup26Match(ctx context.Context, tx *sql.Tx, matchID int, worldCup26MatchID string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE matches
		 SET worldcup26_match_id = $1, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $2 AND (worldcup26_match_id IS NULL OR worldcup26_match_id = $1)`,
		worldCup26MatchID,
		matchID,
	)
	return err
}

func (r *MatchRepository) FindByIDForUpdate(ctx context.Context, tx *sql.Tx, matchID int) (*models.Match, error) {
	var m models.Match
	var homeScore, awayScore sql.NullInt32
	err := tx.QueryRowContext(ctx,
		`SELECT id, round_id, home_team, away_team, home_score, away_score, status, match_time, created_at
		 FROM matches
		 WHERE id = $1
		 FOR UPDATE`,
		matchID,
	).Scan(&m.ID, &m.RoundID, &m.HomeTeam, &m.AwayTeam, &homeScore, &awayScore, &m.Status, &m.MatchTime, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	if homeScore.Valid {
		score := int(homeScore.Int32)
		m.HomeScore = &score
	}
	if awayScore.Valid {
		score := int(awayScore.Int32)
		m.AwayScore = &score
	}
	return &m, nil
}

func (r *MatchRepository) UpdateGuessPoints(ctx context.Context, tx *sql.Tx, guessID, points int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE guesses SET points_earned = $1 WHERE id = $2`,
		points, guessID,
	)
	return err
}

func (r *MatchRepository) AdjustUserPoints(ctx context.Context, tx *sql.Tx, userID, delta int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE users SET total_points = total_points + $1 WHERE id = $2`,
		delta, userID,
	)
	return err
}
