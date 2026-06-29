package services

import (
	"context"
	"database/sql"
	"fmt"

	"backend/internal/models"
)

type ScoreGuessRepository interface {
	FindByMatch(ctx context.Context, tx *sql.Tx, matchID int) ([]models.Guess, error)
}

type ScoreMatchRepository interface {
	FindByIDForUpdate(ctx context.Context, tx *sql.Tx, matchID int) (*models.Match, error)
	UpdateScoreAndStatus(ctx context.Context, tx *sql.Tx, matchID, homeScore, awayScore int) error
	UpdateKnockoutResult(ctx context.Context, tx *sql.Tx, matchID int, winnerTeam, advanceMethod *string) error
	UpdateGuessPoints(ctx context.Context, tx *sql.Tx, guessID, points int) error
	AdjustUserPoints(ctx context.Context, tx *sql.Tx, userID, delta int) error
}

type ScoreUpdateResult struct {
	MatchID             int
	Changed             bool
	GuessesRecalculated int
	PointsDeltaTotal    int
}

type MatchScoreService struct {
	DB      *sql.DB
	Guesses ScoreGuessRepository
	Matches ScoreMatchRepository
}

func NewMatchScoreService(db *sql.DB, guesses ScoreGuessRepository, matches ScoreMatchRepository) *MatchScoreService {
	return &MatchScoreService{DB: db, Guesses: guesses, Matches: matches}
}

func (s *MatchScoreService) UpdateFinalScore(ctx context.Context, matchID, homeScore, awayScore int) (ScoreUpdateResult, error) {
	result := ScoreUpdateResult{MatchID: matchID}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	match, err := s.Matches.FindByIDForUpdate(ctx, tx, matchID)
	if err != nil {
		return result, err
	}
	if match.Status == "finished" && match.HomeScore != nil && match.AwayScore != nil &&
		*match.HomeScore == homeScore && *match.AwayScore == awayScore {
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}

	if err := s.Matches.UpdateScoreAndStatus(ctx, tx, matchID, homeScore, awayScore); err != nil {
		return result, err
	}

	guesses, err := s.Guesses.FindByMatch(ctx, tx, matchID)
	if err != nil {
		return result, err
	}

	result.Changed = true
	result.GuessesRecalculated = len(guesses)

	mc := MatchContext{
		IsKnockout:    match.IsKnockout,
		WinnerTeam:    match.WinnerTeam,
		AdvanceMethod: match.AdvanceMethod,
	}

	for _, g := range guesses {
		gc := GuessContext{
			AdvancingTeam: g.AdvancingTeam,
			AdvanceMethod: g.AdvanceMethod,
		}
		newPoints := CalculatePointsWithContext(g.HomeGuess, g.AwayGuess, homeScore, awayScore, mc, gc)

		oldPoints := 0
		if g.PointsEarned != nil {
			oldPoints = *g.PointsEarned
		}

		if err := s.Matches.UpdateGuessPoints(ctx, tx, g.ID, newPoints); err != nil {
			return result, err
		}

		delta := newPoints - oldPoints
		result.PointsDeltaTotal += delta
		if delta != 0 {
			if err := s.Matches.AdjustUserPoints(ctx, tx, g.UserID, delta); err != nil {
				return result, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}

// UpdateKnockoutResult define winner_team e advance_method para uma partida de mata-mata
// e recalcula os pontos de todos os palpites considerando o bônus de quem avança.
func (s *MatchScoreService) UpdateKnockoutResult(ctx context.Context, matchID int, winnerTeam, advanceMethod *string) (ScoreUpdateResult, error) {
	result := ScoreUpdateResult{MatchID: matchID}

	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	match, err := s.Matches.FindByIDForUpdate(ctx, tx, matchID)
	if err != nil {
		return result, err
	}

	if !match.IsKnockout {
		return result, fmt.Errorf("partida %d não é mata-mata", matchID)
	}

	if err := s.Matches.UpdateKnockoutResult(ctx, tx, matchID, winnerTeam, advanceMethod); err != nil {
		return result, err
	}

	match.WinnerTeam = winnerTeam
	match.AdvanceMethod = advanceMethod

	guesses, err := s.Guesses.FindByMatch(ctx, tx, matchID)
	if err != nil {
		return result, err
	}

	result.Changed = true
	result.GuessesRecalculated = len(guesses)

	homeScore := 0
	awayScore := 0
	if match.HomeScore != nil {
		homeScore = *match.HomeScore
	}
	if match.AwayScore != nil {
		awayScore = *match.AwayScore
	}

	mc := MatchContext{
		IsKnockout:    match.IsKnockout,
		WinnerTeam:    match.WinnerTeam,
		AdvanceMethod: match.AdvanceMethod,
	}

	for _, g := range guesses {
		gc := GuessContext{
			AdvancingTeam: g.AdvancingTeam,
			AdvanceMethod: g.AdvanceMethod,
		}
		newPoints := CalculatePointsWithContext(g.HomeGuess, g.AwayGuess, homeScore, awayScore, mc, gc)

		oldPoints := 0
		if g.PointsEarned != nil {
			oldPoints = *g.PointsEarned
		}

		if err := s.Matches.UpdateGuessPoints(ctx, tx, g.ID, newPoints); err != nil {
			return result, err
		}

		delta := newPoints - oldPoints
		result.PointsDeltaTotal += delta
		if delta != 0 {
			if err := s.Matches.AdjustUserPoints(ctx, tx, g.UserID, delta); err != nil {
				return result, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}

	return result, nil
}
