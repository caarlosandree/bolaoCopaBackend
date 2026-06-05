package repositories

import (
	"context"
	"database/sql"

	"backend/internal/models"
)

type GuessRepository struct {
	DB *sql.DB
}

func NewGuessRepository(db *sql.DB) *GuessRepository {
	return &GuessRepository{DB: db}
}

func (r *GuessRepository) Upsert(ctx context.Context, userID, matchID, homeGuess, awayGuess int) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO guesses (user_id, match_id, home_guess, away_guess)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, match_id)
		 DO UPDATE SET home_guess = EXCLUDED.home_guess, away_guess = EXCLUDED.away_guess`,
		userID, matchID, homeGuess, awayGuess,
	)
	return err
}

func (r *GuessRepository) FindByMatch(ctx context.Context, tx *sql.Tx, matchID int) ([]models.Guess, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, user_id, home_guess, away_guess, points_earned
		 FROM guesses WHERE match_id = $1`,
		matchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var guesses []models.Guess
	for rows.Next() {
		var g models.Guess
		var pts sql.NullInt32
		if err := rows.Scan(&g.ID, &g.UserID, &g.HomeGuess, &g.AwayGuess, &pts); err != nil {
			return nil, err
		}
		if pts.Valid {
			p := int(pts.Int32)
			g.PointsEarned = &p
		}
		guesses = append(guesses, g)
	}
	return guesses, rows.Err()
}
