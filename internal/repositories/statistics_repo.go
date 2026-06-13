package repositories

import (
	"context"
	"database/sql"
)

type StatisticsRepository struct {
	DB *sql.DB
}

func NewStatisticsRepository(db *sql.DB) *StatisticsRepository {
	return &StatisticsRepository{DB: db}
}

type CopaLocalStats struct {
	TotalGoals int
	BiggestWin *BiggestWin
}

type BiggestWin struct {
	HomeTeam  string
	AwayTeam  string
	HomeScore int
	AwayScore int
	MatchTime string
}

func (r *StatisticsRepository) GetCopaLocalStats(ctx context.Context) (CopaLocalStats, error) {
	var stats CopaLocalStats

	row := r.DB.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(home_score + away_score), 0)
		 FROM matches
		 WHERE status = 'finished' AND home_score IS NOT NULL AND away_score IS NOT NULL`,
	)
	if err := row.Scan(&stats.TotalGoals); err != nil {
		return stats, err
	}

	bwRow := r.DB.QueryRowContext(ctx,
		`SELECT home_team, away_team, home_score, away_score,
		        to_char(match_time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM matches
		 WHERE status = 'finished' AND home_score IS NOT NULL AND away_score IS NOT NULL
		 ORDER BY ABS(home_score - away_score) DESC, (home_score + away_score) DESC
		 LIMIT 1`,
	)
	var bw BiggestWin
	err := bwRow.Scan(&bw.HomeTeam, &bw.AwayTeam, &bw.HomeScore, &bw.AwayScore, &bw.MatchTime)
	if err == nil {
		stats.BiggestWin = &bw
	} else if err != sql.ErrNoRows {
		return stats, err
	}

	return stats, nil
}

type BolaoOverviewStats struct {
	TotalGuesses int
	ExactHits    int
	HitRate      float64
	BestRound    *BestRound
}

type BestRound struct {
	Name        string
	TotalPoints int
}

func (r *StatisticsRepository) GetBolaoOverview(ctx context.Context) (BolaoOverviewStats, error) {
	var stats BolaoOverviewStats

	row := r.DB.QueryRowContext(ctx,
		`SELECT
		   COUNT(*),
		   COUNT(*) FILTER (WHERE points_earned = 5),
		   CASE WHEN COUNT(*) FILTER (WHERE points_earned IS NOT NULL) = 0 THEN 0
		        ELSE ROUND(COUNT(*) FILTER (WHERE points_earned > 0)::numeric /
		                   COUNT(*) FILTER (WHERE points_earned IS NOT NULL), 4)
		   END
		 FROM guesses`,
	)
	if err := row.Scan(&stats.TotalGuesses, &stats.ExactHits, &stats.HitRate); err != nil {
		return stats, err
	}

	brRow := r.DB.QueryRowContext(ctx,
		`SELECT r.name, SUM(g.points_earned) AS total
		 FROM guesses g
		 JOIN matches m ON m.id = g.match_id
		 JOIN rounds r ON r.id = m.round_id
		 WHERE g.points_earned IS NOT NULL
		 GROUP BY r.id, r.name
		 ORDER BY total DESC
		 LIMIT 1`,
	)
	var br BestRound
	if err := brRow.Scan(&br.Name, &br.TotalPoints); err == nil {
		stats.BestRound = &br
	}

	return stats, nil
}

type UserRoundPoints struct {
	UserID      int
	Name        string
	AvatarURL   *string
	RoundNumber int
	RoundPoints int
}

func (r *StatisticsRepository) GetPointsEvolution(ctx context.Context) ([]UserRoundPoints, error) {
	rows, err := r.DB.QueryContext(ctx,
		`WITH top5 AS (
		   SELECT id, name, avatar_url
		   FROM users
		   ORDER BY total_points DESC
		   LIMIT 5
		 )
		 SELECT t.id, t.name, t.avatar_url, ro.number,
		        COALESCE(SUM(g.points_earned), 0) AS round_points
		 FROM top5 t
		 CROSS JOIN rounds ro
		 LEFT JOIN matches m ON m.round_id = ro.id
		 LEFT JOIN guesses g ON g.match_id = m.id AND g.user_id = t.id AND g.points_earned IS NOT NULL
		 GROUP BY t.id, t.name, t.avatar_url, ro.id, ro.number
		 ORDER BY t.id, ro.number`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []UserRoundPoints
	for rows.Next() {
		var u UserRoundPoints
		if err := rows.Scan(&u.UserID, &u.Name, &u.AvatarURL, &u.RoundNumber, &u.RoundPoints); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

type GuessCount struct {
	HomeGuess int
	AwayGuess int
	Total     int
}

func (r *StatisticsRepository) GetGuessDistribution(ctx context.Context) ([]GuessCount, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT home_guess, away_guess, COUNT(*) AS total
		 FROM guesses
		 GROUP BY home_guess, away_guess
		 ORDER BY total DESC
		 LIMIT 8`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []GuessCount
	for rows.Next() {
		var g GuessCount
		if err := rows.Scan(&g.HomeGuess, &g.AwayGuess, &g.Total); err != nil {
			return nil, err
		}
		result = append(result, g)
	}
	return result, rows.Err()
}

type AccuracyRow struct {
	Name      string
	AvatarURL *string
	Exact     int
	Partial   int
	Wrong     int
	Total     int
}

func (r *StatisticsRepository) GetAccuracyRanking(ctx context.Context) ([]AccuracyRow, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT
		   u.name,
		   u.avatar_url,
		   COUNT(*) FILTER (WHERE g.points_earned = 5)       AS exact,
		   COUNT(*) FILTER (WHERE g.points_earned IN (2, 3)) AS partial,
		   COUNT(*) FILTER (WHERE g.points_earned = 0)       AS wrong,
		   COUNT(*) FILTER (WHERE g.points_earned IS NOT NULL) AS total
		 FROM guesses g
		 JOIN users u ON u.id = g.user_id
		 WHERE g.points_earned IS NOT NULL
		 GROUP BY u.id, u.name, u.avatar_url
		 ORDER BY exact DESC, partial DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []AccuracyRow
	for rows.Next() {
		var a AccuracyRow
		if err := rows.Scan(&a.Name, &a.AvatarURL, &a.Exact, &a.Partial, &a.Wrong, &a.Total); err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetActiveMatchExternalIDs retorna os IDs externos (TheSportsDB) das partidas em andamento.
func (r *StatisticsRepository) GetActiveMatchExternalIDs(ctx context.Context) ([]string, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT thesportsdb_event_id
		 FROM matches
		 WHERE status = 'ongoing' AND thesportsdb_event_id IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
