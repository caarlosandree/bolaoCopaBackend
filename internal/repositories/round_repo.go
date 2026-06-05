package repositories

import (
	"context"
	"database/sql"

	"backend/internal/models"
)

type RoundRepository struct {
	DB *sql.DB
}

func NewRoundRepository(db *sql.DB) *RoundRepository {
	return &RoundRepository{DB: db}
}

func (r *RoundRepository) FindActive(ctx context.Context) (*models.Round, error) {
	var round models.Round
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, tournament_id, number, name, status, created_at
		 FROM rounds WHERE status = 'active' LIMIT 1`,
	).Scan(&round.ID, &round.TournamentID, &round.Number, &round.Name, &round.Status, &round.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &round, nil
}

func (r *RoundRepository) ListAll(ctx context.Context) ([]models.Round, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, tournament_id, number, name, status, created_at
		 FROM rounds
		 ORDER BY number ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []models.Round
	for rows.Next() {
		var round models.Round
		if err := rows.Scan(&round.ID, &round.TournamentID, &round.Number, &round.Name, &round.Status, &round.CreatedAt); err != nil {
			return nil, err
		}
		rounds = append(rounds, round)
	}
	return rounds, rows.Err()
}

func (r *RoundRepository) FindMatchesByRound(ctx context.Context, roundID, userID int) ([]models.Match, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT m.id, m.round_id, m.home_team, m.away_team,
		        m.home_score, m.away_score, m.status, m.match_time,
		        m.group_name, m.venue,
		        g.id, g.home_guess, g.away_guess, g.points_earned
		 FROM matches m
		 LEFT JOIN guesses g ON g.match_id = m.id AND g.user_id = $2
		 WHERE m.round_id = $1
		 ORDER BY m.match_time ASC`,
		roundID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []models.Match
	for rows.Next() {
		var m models.Match
		var groupName, venue sql.NullString
		var guessID sql.NullInt64
		var homeGuess, awayGuess sql.NullInt32
		var pointsEarned sql.NullInt32

		err := rows.Scan(
			&m.ID, &m.RoundID, &m.HomeTeam, &m.AwayTeam,
			&m.HomeScore, &m.AwayScore, &m.Status, &m.MatchTime,
			&groupName, &venue,
			&guessID, &homeGuess, &awayGuess, &pointsEarned,
		)
		if err != nil {
			return nil, err
		}

		if groupName.Valid {
			m.GroupName = &groupName.String
		}
		if venue.Valid {
			m.Venue = &venue.String
		}

		if guessID.Valid {
			ug := &models.UserGuess{
				ID:        int(guessID.Int64),
				HomeGuess: int(homeGuess.Int32),
				AwayGuess: int(awayGuess.Int32),
			}
			if pointsEarned.Valid {
				pts := int(pointsEarned.Int32)
				ug.PointsEarned = &pts
			}
			m.UserGuess = ug
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}
