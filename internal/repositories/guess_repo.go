package repositories

import (
	"context"
	"database/sql"
	"errors"

	"backend/internal/models"
)

type GuessRepository struct {
	DB *sql.DB
}

func NewGuessRepository(db *sql.DB) *GuessRepository {
	return &GuessRepository{DB: db}
}

func (r *GuessRepository) Upsert(ctx context.Context, userID, matchID, homeGuess, awayGuess int, advancingTeam, advanceMethod *string) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO guesses (user_id, match_id, home_guess, away_guess, advancing_team, advance_method)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (user_id, match_id)
		 DO UPDATE SET home_guess = EXCLUDED.home_guess, away_guess = EXCLUDED.away_guess,
		    advancing_team = EXCLUDED.advancing_team, advance_method = EXCLUDED.advance_method`,
		userID, matchID, homeGuess, awayGuess, advancingTeam, advanceMethod,
	)
	return err
}

// FindByUserAndMatch retorna o palpite do usuário para a partida, ou nil se não existir.
func (r *GuessRepository) FindByUserAndMatch(ctx context.Context, userID, matchID int) (*models.Guess, error) {
	var g models.Guess
	var pts sql.NullInt32
	var advancingTeam, advanceMethod sql.NullString
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, user_id, match_id, home_guess, away_guess, points_earned, advancing_team, advance_method
		 FROM guesses WHERE user_id = $1 AND match_id = $2`,
		userID, matchID,
	).Scan(
		&g.ID, &g.UserID, &g.MatchID, &g.HomeGuess, &g.AwayGuess, &pts,
		&advancingTeam, &advanceMethod,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if pts.Valid {
		p := int(pts.Int32)
		g.PointsEarned = &p
	}
	if advancingTeam.Valid && advancingTeam.String != "" {
		v := advancingTeam.String
		g.AdvancingTeam = &v
	}
	if advanceMethod.Valid && advanceMethod.String != "" {
		v := advanceMethod.String
		g.AdvanceMethod = &v
	}
	return &g, nil
}

func (r *GuessRepository) FindByMatchWithUsers(ctx context.Context, matchID int) ([]models.MatchGuessView, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT u.id, u.name, u.avatar_url, g.home_guess, g.away_guess, g.points_earned,
		        g.advancing_team, g.advance_method
		 FROM guesses g
		 JOIN users u ON u.id = g.user_id
		 WHERE g.match_id = $1
		 ORDER BY g.points_earned DESC NULLS LAST, u.name ASC`,
		matchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []models.MatchGuessView
	for rows.Next() {
		var v models.MatchGuessView
		var pts sql.NullInt32
		var avatarURL, advancingTeam, advanceMethod sql.NullString
		if err := rows.Scan(&v.UserID, &v.Name, &avatarURL, &v.HomeGuess, &v.AwayGuess, &pts,
			&advancingTeam, &advanceMethod); err != nil {
			return nil, err
		}
		if pts.Valid {
			p := int(pts.Int32)
			v.PointsEarned = &p
		}
		if avatarURL.Valid {
			v.AvatarURL = &avatarURL.String
		}
		if advancingTeam.Valid && advancingTeam.String != "" {
			v.AdvancingTeam = &advancingTeam.String
		}
		if advanceMethod.Valid && advanceMethod.String != "" {
			v.AdvanceMethod = &advanceMethod.String
		}
		views = append(views, v)
	}
	return views, rows.Err()
}

func (r *GuessRepository) FindByMatch(ctx context.Context, tx *sql.Tx, matchID int) ([]models.Guess, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, user_id, home_guess, away_guess, points_earned, advancing_team, advance_method
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
		var advancingTeam, advanceMethod sql.NullString
		if err := rows.Scan(&g.ID, &g.UserID, &g.HomeGuess, &g.AwayGuess, &pts,
			&advancingTeam, &advanceMethod); err != nil {
			return nil, err
		}
		if pts.Valid {
			p := int(pts.Int32)
			g.PointsEarned = &p
		}
		if advancingTeam.Valid && advancingTeam.String != "" {
			v := advancingTeam.String
			g.AdvancingTeam = &v
		}
		if advanceMethod.Valid && advanceMethod.String != "" {
			v := advanceMethod.String
			g.AdvanceMethod = &v
		}
		guesses = append(guesses, g)
	}
	return guesses, rows.Err()
}
