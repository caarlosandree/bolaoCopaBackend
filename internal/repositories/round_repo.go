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

func (r *RoundRepository) ListSummary(ctx context.Context) ([]models.RoundSummary, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT r.id, r.tournament_id, r.number, r.name, r.status,
		        COUNT(m.id) AS match_count,
		        MIN(m.match_time) AS first_match_at,
		        MAX(m.match_time) AS last_match_at
		 FROM rounds r
		 LEFT JOIN matches m ON m.round_id = r.id
		 GROUP BY r.id
		 HAVING COUNT(m.id) > 0 AND r.number <> 122
		 ORDER BY r.number ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rounds []models.RoundSummary
	for rows.Next() {
		var s models.RoundSummary
		var firstAt, lastAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.TournamentID, &s.Number, &s.Name, &s.Status,
			&s.MatchCount, &firstAt, &lastAt); err != nil {
			return nil, err
		}
		if firstAt.Valid {
			s.FirstMatchAt = &firstAt.Time
		}
		if lastAt.Valid {
			s.LastMatchAt = &lastAt.Time
		}
		rounds = append(rounds, s)
	}
	return rounds, rows.Err()
}

func (r *RoundRepository) FindByID(ctx context.Context, roundID int) (*models.Round, error) {
	var round models.Round
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, tournament_id, number, name, status, created_at
		 FROM rounds WHERE id = $1`,
		roundID,
	).Scan(&round.ID, &round.TournamentID, &round.Number, &round.Name, &round.Status, &round.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &round, nil
}

func (r *RoundRepository) ActivateDueRounds(ctx context.Context) (int, error) {
	res, err := r.DB.ExecContext(ctx,
		`UPDATE rounds SET status = 'active'
		 WHERE status = 'upcoming'
		   AND id IN (
		       SELECT round_id FROM matches
		       GROUP BY round_id
		       HAVING MIN(match_time) <= NOW() + INTERVAL '24 hours'
		   )`,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *RoundRepository) FinishCompletedRounds(ctx context.Context) (int, error) {
	res, err := r.DB.ExecContext(ctx,
		`UPDATE rounds SET status = 'finished'
		 WHERE status = 'active'
		   AND id NOT IN (
		       SELECT round_id FROM matches
		       WHERE status <> 'finished'
		   )
		   AND id IN (SELECT round_id FROM matches)`,
	)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (r *RoundRepository) SetStatus(ctx context.Context, roundID int, status string) error {
	res, err := r.DB.ExecContext(ctx,
		`UPDATE rounds SET status = $1 WHERE id = $2`,
		status, roundID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *RoundRepository) FindMatchesByRound(ctx context.Context, roundID, userID int) ([]models.Match, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT m.id, m.round_id, m.home_team, m.away_team,
		        m.home_score, m.away_score, m.status, m.match_time,
		        m.group_name, m.venue, m.stream_url,
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
		var groupName, venue, streamURL sql.NullString
		var guessID sql.NullInt64
		var homeGuess, awayGuess sql.NullInt32
		var pointsEarned sql.NullInt32

		err := rows.Scan(
			&m.ID, &m.RoundID, &m.HomeTeam, &m.AwayTeam,
			&m.HomeScore, &m.AwayScore, &m.Status, &m.MatchTime,
			&groupName, &venue, &streamURL,
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
		if streamURL.Valid {
			m.StreamURL = &streamURL.String
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
