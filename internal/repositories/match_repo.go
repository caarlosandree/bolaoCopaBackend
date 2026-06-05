package repositories

import (
	"context"
	"database/sql"
	"time"
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
		`UPDATE matches SET home_score = $1, away_score = $2, status = 'finished' WHERE id = $3`,
		homeScore, awayScore, matchID,
	)
	return err
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
