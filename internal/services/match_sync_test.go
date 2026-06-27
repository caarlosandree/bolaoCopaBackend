package services

import (
	"testing"
	"time"
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

func TestParseIntRoundMapsKnockoutRounds(t *testing.T) {
	t.Parallel()

	tests := map[string]int{
		"1":  1,
		"2":  2,
		"3":  3,
		"32": 100,
		"16": 101,
		"4":  100,
		"5":  101,
		"6":  102,
		"7":  103,
		"8":  104,
		"9":  105,
	}

	for input, want := range tests {
		input := input
		want := want

		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := parseIntRound(input); got != want {
				t.Fatalf("parseIntRound(%q) = %d, want %d", input, got, want)
			}
		})
	}
}
