package services

import (
	"testing"
	"time"

	"backend/internal/repositories"
)

func TestParseTheSportsDBTimestamp(t *testing.T) {
	got, err := parseTheSportsDBTimestamp("2026-06-11T19:00:00")
	if err != nil {
		t.Fatalf("parseTheSportsDBTimestamp returned error: %v", err)
	}

	want := time.Date(2026, 6, 11, 19, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("expected %s, got %s", want, got)
	}
}


func TestFindWorldCup26MatchNormalizesTeamNames(t *testing.T) {
	group := "Group B"
	matches := []repositories.MatchSyncRow{
		{
			ID:        10,
			HomeTeam:  "Canada",
			AwayTeam:  "Bosnia & Herzegovina",
			GroupName: &group,
		},
	}

	got, ok := findWorldCup26Match(matches, worldCup26Game{
		ID:             "3",
		Group:          "B",
		HomeTeamNameEN: "Canada",
		AwayTeamNameEN: "Bosnia and Herzegovina",
	})
	if !ok {
		t.Fatal("expected match to be found")
	}
	if got.ID != 10 {
		t.Fatalf("expected match ID 10, got %d", got.ID)
	}
}
