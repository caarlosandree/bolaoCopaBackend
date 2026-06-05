package services

import (
	"context"
	"database/sql"

	"backend/internal/models"
)

type ScoreGuessRepository interface {
	FindByMatch(ctx context.Context, tx *sql.Tx, matchID int) ([]models.Guess, error)
}

type ScoreMatchRepository interface {
	FindByIDForUpdate(ctx context.Context, tx *sql.Tx, matchID int) (*models.Match, error)
	UpdateScoreAndStatus(ctx context.Context, tx *sql.Tx, matchID, homeScore, awayScore int) error
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
	for _, g := range guesses {
		newPoints := CalculatePoints(g.HomeGuess, g.AwayGuess, homeScore, awayScore)

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
