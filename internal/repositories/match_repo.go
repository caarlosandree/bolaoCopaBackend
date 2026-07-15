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
	MatchTime  time.Time
	Status     string
	IsKnockout bool
}

type ImportedMatch struct {
	ExternalSource     string
	ExternalID         string
	TheSportsDBEventID *string
	TheSportsDBHomeID  *string
	TheSportsDBAwayID  *string
	TournamentName     string
	RoundNumber        int
	RoundName          string
	HomeTeam           string
	AwayTeam           string
	MatchTime          time.Time
	GroupName          *string
	Venue              *string
	MatchNumber        *int
	IsKnockout         bool
}

type MatchSyncRow struct {
	ID                 int
	HomeTeam           string
	AwayTeam           string
	HomeScore          *int
	AwayScore          *int
	Status             string
	MatchTime          time.Time
	GroupName          *string
	TheSportsDBEventID *string
	IsKnockout         bool
	WinnerTeam         *string
	AdvanceMethod      *string
}

func (r *MatchRepository) FindMatchTime(ctx context.Context, matchID int) (*MatchTime, error) {
	var m MatchTime
	err := r.DB.QueryRowContext(ctx,
		`SELECT match_time, status, is_knockout FROM matches WHERE id = $1`,
		matchID,
	).Scan(&m.MatchTime, &m.Status, &m.IsKnockout)
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

	if m.TheSportsDBEventID != nil {
		matchID, err := r.updateMatchingImported(ctx, tx, m, roundID)
		if err != nil {
			return 0, err
		}
		if matchID != 0 {
			return matchID, nil
		}
	}

	var matchID int
	err = tx.QueryRowContext(ctx,
		`INSERT INTO matches (
		    round_id, home_team, away_team, match_time, status,
		    external_source, external_id, group_name, venue, match_number,
		    thesportsdb_event_id, thesportsdb_home_team_id, thesportsdb_away_team_id,
		    is_knockout
		 )
		 VALUES ($1, $2, $3, $4, 'scheduled', $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT (external_source, external_id)
		 WHERE external_source IS NOT NULL AND external_id IS NOT NULL
		 DO UPDATE SET
		    round_id = EXCLUDED.round_id,
		    home_team = COALESCE(NULLIF(EXCLUDED.home_team, ''), matches.home_team),
		    away_team = COALESCE(NULLIF(EXCLUDED.away_team, ''), matches.away_team),
		    match_time = EXCLUDED.match_time,
		    group_name = EXCLUDED.group_name,
		    venue = EXCLUDED.venue,
		    match_number = EXCLUDED.match_number,
		    thesportsdb_event_id = COALESCE(matches.thesportsdb_event_id, EXCLUDED.thesportsdb_event_id),
		    thesportsdb_home_team_id = COALESCE(matches.thesportsdb_home_team_id, EXCLUDED.thesportsdb_home_team_id),
		    thesportsdb_away_team_id = COALESCE(matches.thesportsdb_away_team_id, EXCLUDED.thesportsdb_away_team_id),
		    is_knockout = EXCLUDED.is_knockout,
		    updated_at = CURRENT_TIMESTAMP
		 RETURNING id`,
		roundID,
		m.HomeTeam,
		m.AwayTeam,
		m.MatchTime,
		m.ExternalSource,
		m.ExternalID,
		m.GroupName,
		m.Venue,
		m.MatchNumber,
		m.TheSportsDBEventID,
		m.TheSportsDBHomeID,
		m.TheSportsDBAwayID,
		m.IsKnockout,
	).Scan(&matchID)
	if err != nil {
		return 0, err
	}
	return matchID, nil
}

func (r *MatchRepository) updateMatchingImported(ctx context.Context, tx *sql.Tx, m ImportedMatch, roundID int) (int, error) {
	// Se já existe um match com esse thesportsdb_event_id, atualiza os campos
	// incrementais do evento sem tentar atribuir o ID a outro match.
	var existing int
	err := tx.QueryRowContext(ctx,
		`UPDATE matches
		 SET round_id = $2,
		     home_team = COALESCE(NULLIF($3, ''), home_team),
		     away_team = COALESCE(NULLIF($4, ''), away_team),
		     match_time = $5,
		     group_name = COALESCE($6, group_name),
		     venue = COALESCE($7, venue),
		     match_number = COALESCE($8, match_number),
		     thesportsdb_home_team_id = COALESCE(thesportsdb_home_team_id, $9),
		     thesportsdb_away_team_id = COALESCE(thesportsdb_away_team_id, $10),
		     is_knockout = $11,
		     updated_at = CURRENT_TIMESTAMP
		 WHERE thesportsdb_event_id = $1
		 RETURNING id`,
		m.TheSportsDBEventID,
		roundID,
		m.HomeTeam,
		m.AwayTeam,
		m.MatchTime,
		m.GroupName,
		m.Venue,
		m.MatchNumber,
		m.TheSportsDBHomeID,
		m.TheSportsDBAwayID,
		m.IsKnockout,
	).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	if existing != 0 {
		return existing, nil
	}

	var matchID int
	err = tx.QueryRowContext(ctx,
		`WITH candidate AS (
		    SELECT id
		    FROM matches
		    WHERE lower(home_team) = lower($8)
		      AND lower(away_team) = lower($9)
		      AND match_time = $10
		      AND (
		          thesportsdb_event_id IS NULL
		          OR thesportsdb_event_id = $4
		      )
		    ORDER BY id ASC
		    LIMIT 1
		 )
		 UPDATE matches
		 SET group_name = COALESCE($1, group_name),
		     venue = COALESCE($2, venue),
		     match_number = COALESCE($3, match_number),
		     thesportsdb_event_id = COALESCE(thesportsdb_event_id, $4),
		     thesportsdb_home_team_id = COALESCE(thesportsdb_home_team_id, $5),
		     thesportsdb_away_team_id = COALESCE(thesportsdb_away_team_id, $6),
		     is_knockout = $7,
		     updated_at = CURRENT_TIMESTAMP
		 FROM candidate
		 WHERE matches.id = candidate.id
		 RETURNING matches.id`,
		m.GroupName,
		m.Venue,
		m.MatchNumber,
		m.TheSportsDBEventID,
		m.TheSportsDBHomeID,
		m.TheSportsDBAwayID,
		m.IsKnockout,
		m.HomeTeam,
		m.AwayTeam,
		m.MatchTime,
	).Scan(&matchID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return matchID, nil
}

type DetailsSyncMatch struct {
	ID                 int
	HomeTeam           string
	AwayTeam           string
	Status             string
	MatchTime          time.Time
	TheSportsDBEventID *string
	TheSportsDBHomeID  *string
	TheSportsDBAwayID  *string
}

func (r *MatchRepository) ListForDetailsSync(ctx context.Context) ([]DetailsSyncMatch, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, home_team, away_team, status, match_time,
		        thesportsdb_event_id, thesportsdb_home_team_id, thesportsdb_away_team_id
		 FROM matches
		 ORDER BY match_time ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []DetailsSyncMatch
	for rows.Next() {
		var m DetailsSyncMatch
		var eventID, homeID, awayID sql.NullString
		if err := rows.Scan(
			&m.ID,
			&m.HomeTeam,
			&m.AwayTeam,
			&m.Status,
			&m.MatchTime,
			&eventID,
			&homeID,
			&awayID,
		); err != nil {
			return nil, err
		}
		m.TheSportsDBEventID = stringPtrIfValid(eventID)
		m.TheSportsDBHomeID = stringPtrIfValid(homeID)
		m.TheSportsDBAwayID = stringPtrIfValid(awayID)
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func stringPtrIfValid(value sql.NullString) *string {
	if value.Valid && value.String != "" {
		v := value.String
		return &v
	}
	return nil
}

func (r *MatchRepository) ListForSync(ctx context.Context) ([]MatchSyncRow, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, home_team, away_team, home_score, away_score, status, match_time, group_name, thesportsdb_event_id,
		        is_knockout, winner_team, advance_method
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
		var groupName, theSportsDBEventID, winnerTeam, advanceMethod sql.NullString
		if err := rows.Scan(
			&m.ID,
			&m.HomeTeam,
			&m.AwayTeam,
			&homeScore,
			&awayScore,
			&m.Status,
			&m.MatchTime,
			&groupName,
			&theSportsDBEventID,
			&m.IsKnockout,
			&winnerTeam,
			&advanceMethod,
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
		if theSportsDBEventID.Valid {
			value := theSportsDBEventID.String
			m.TheSportsDBEventID = &value
		}
		if winnerTeam.Valid && winnerTeam.String != "" {
			value := winnerTeam.String
			m.WinnerTeam = &value
		}
		if advanceMethod.Valid && advanceMethod.String != "" {
			value := advanceMethod.String
			m.AdvanceMethod = &value
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

type AdminMatch struct {
	ID            int       `json:"id"`
	HomeTeam      string    `json:"home_team"`
	AwayTeam      string    `json:"away_team"`
	HomeScore     *int      `json:"home_score"`
	AwayScore     *int      `json:"away_score"`
	Status        string    `json:"status"`
	MatchTime     time.Time `json:"match_time"`
	RoundName     string    `json:"round_name"`
	GroupName     *string   `json:"group_name"`
	IsKnockout    bool      `json:"is_knockout"`
	WinnerTeam    *string   `json:"winner_team,omitempty"`
	AdvanceMethod *string   `json:"advance_method,omitempty"`
}

type AdminMatchesPage struct {
	Items      []AdminMatch `json:"items"`
	Total      int          `json:"total"`
	Page       int          `json:"page"`
	PageSize   int          `json:"page_size"`
	TotalPages int          `json:"total_pages"`
}

func (r *MatchRepository) ListAdminPage(ctx context.Context, page, pageSize int) (*AdminMatchesPage, error) {
	var total int
	if err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM matches`).Scan(&total); err != nil {
		return nil, err
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page > totalPages && totalPages > 0 {
		page = totalPages
	}
	offset := (page - 1) * pageSize

	rows, err := r.DB.QueryContext(ctx,
		`SELECT m.id, m.home_team, m.away_team, m.home_score, m.away_score, m.status,
		        m.match_time, r.name AS round_name, m.group_name,
		        m.is_knockout, m.winner_team, m.advance_method
		 FROM matches m
		 JOIN rounds r ON r.id = m.round_id
		 ORDER BY m.match_time ASC, m.id ASC
		 LIMIT $1 OFFSET $2`,
		pageSize,
		offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	matches := []AdminMatch{}
	for rows.Next() {
		var m AdminMatch
		var homeScore, awayScore sql.NullInt32
		var groupName, winnerTeam, advanceMethod sql.NullString
		if err := rows.Scan(
			&m.ID, &m.HomeTeam, &m.AwayTeam,
			&homeScore, &awayScore,
			&m.Status, &m.MatchTime, &m.RoundName, &groupName,
			&m.IsKnockout, &winnerTeam, &advanceMethod,
		); err != nil {
			return nil, err
		}
		if homeScore.Valid {
			v := int(homeScore.Int32)
			m.HomeScore = &v
		}
		if awayScore.Valid {
			v := int(awayScore.Int32)
			m.AwayScore = &v
		}
		if groupName.Valid {
			m.GroupName = &groupName.String
		}
		if winnerTeam.Valid && winnerTeam.String != "" {
			v := winnerTeam.String
			m.WinnerTeam = &v
		}
		if advanceMethod.Valid && advanceMethod.String != "" {
			v := advanceMethod.String
			m.AdvanceMethod = &v
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &AdminMatchesPage{
		Items:      matches,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}, nil
}

func (r *MatchRepository) FindByID(ctx context.Context, matchID int) (*models.Match, error) {
	var m models.Match
	var homeScore, awayScore sql.NullInt32
	var winnerTeam, advanceMethod sql.NullString
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, round_id, home_team, away_team, home_score, away_score, status, match_time,
		        is_knockout, winner_team, advance_method
		 FROM matches WHERE id = $1`,
		matchID,
	).Scan(&m.ID, &m.RoundID, &m.HomeTeam, &m.AwayTeam, &homeScore, &awayScore, &m.Status, &m.MatchTime,
		&m.IsKnockout, &winnerTeam, &advanceMethod)
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
	if winnerTeam.Valid && winnerTeam.String != "" {
		v := winnerTeam.String
		m.WinnerTeam = &v
	}
	if advanceMethod.Valid && advanceMethod.String != "" {
		v := advanceMethod.String
		m.AdvanceMethod = &v
	}
	return &m, nil
}

func (r *MatchRepository) FindByIDForUpdate(ctx context.Context, tx *sql.Tx, matchID int) (*models.Match, error) {
	var m models.Match
	var homeScore, awayScore sql.NullInt32
	var winnerTeam, advanceMethod sql.NullString
	err := tx.QueryRowContext(ctx,
		`SELECT id, round_id, home_team, away_team, home_score, away_score, status, match_time, created_at,
		        is_knockout, winner_team, advance_method
		 FROM matches
		 WHERE id = $1
		 FOR UPDATE`,
		matchID,
	).Scan(&m.ID, &m.RoundID, &m.HomeTeam, &m.AwayTeam, &homeScore, &awayScore, &m.Status, &m.MatchTime, &m.CreatedAt,
		&m.IsKnockout, &winnerTeam, &advanceMethod)
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
	if winnerTeam.Valid && winnerTeam.String != "" {
		v := winnerTeam.String
		m.WinnerTeam = &v
	}
	if advanceMethod.Valid && advanceMethod.String != "" {
		v := advanceMethod.String
		m.AdvanceMethod = &v
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

// UpdateKnockoutResult define winner_team e advance_method para uma partida de mata-mata.
// Usado pelo admin quando o jogo foi decidido na prorrogação ou pênaltis.
func (r *MatchRepository) UpdateKnockoutResult(ctx context.Context, tx *sql.Tx, matchID int, winnerTeam, advanceMethod *string) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE matches
		 SET winner_team = $1, advance_method = $2, updated_at = CURRENT_TIMESTAMP
		 WHERE id = $3`,
		winnerTeam, advanceMethod, matchID,
	)
	return err
}

// SetKnockoutFlag marca ou desmarca uma partida como mata-mata.
func (r *MatchRepository) SetKnockoutFlag(ctx context.Context, matchID int, isKnockout bool) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE matches SET is_knockout = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
		isKnockout, matchID,
	)
	return err
}

type OngoingMatch struct {
	ID       int
	HomeTeam string
	AwayTeam string
}

func (r *MatchRepository) FindOngoing(ctx context.Context) ([]OngoingMatch, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, home_team, away_team
		 FROM matches
		 WHERE status = 'ongoing'
		    OR (status = 'scheduled' AND match_time <= NOW())`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []OngoingMatch
	for rows.Next() {
		var m OngoingMatch
		if err := rows.Scan(&m.ID, &m.HomeTeam, &m.AwayTeam); err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

func (r *MatchRepository) UpdateStreamURL(ctx context.Context, matchID int, streamURL *string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE matches SET stream_url = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2`,
		streamURL, matchID,
	)
	return err
}

// ResetSchedule apaga todos os rounds e seus jogos (cascata via FK).
// Zera total_points de todos os usuários e remove todos os palpites.
func (r *MatchRepository) ResetSchedule(ctx context.Context) error {
	tx, err := r.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, q := range []string{
		`DELETE FROM guesses`,
		`UPDATE users SET total_points = 0`,
		`DELETE FROM matches`,
		`DELETE FROM rounds`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return tx.Commit()
}
