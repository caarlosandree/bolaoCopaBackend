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

func TestTheSportsDBStatusToInternalFinishedVariants(t *testing.T) {
	t.Parallel()

	// AET/AP são os status que o TheSportsDB usa em mata-mata após prorrogação/pênaltis.
	// Antes caíam no default e viravam "scheduled", bloqueando pontuação e fechamento de rodada.
	finished := []string{"FT", "ft", "AET", "aet", "AP", "ap", "PEN", "Match Finished"}
	for _, st := range finished {
		if got := theSportsDBStatusToInternal(st); got != "finished" {
			t.Fatalf("theSportsDBStatusToInternal(%q) = %q, want finished", st, got)
		}
	}

	ongoing := []string{"1H", "HT", "2H", "ET", "P", "Live"}
	for _, st := range ongoing {
		if got := theSportsDBStatusToInternal(st); got != "ongoing" {
			t.Fatalf("theSportsDBStatusToInternal(%q) = %q, want ongoing", st, got)
		}
	}
}

func TestTheSportsDBAdvanceMethod(t *testing.T) {
	t.Parallel()

	if got := theSportsDBAdvanceMethod("AET"); got != "et" {
		t.Fatalf("AET -> %q, want et", got)
	}
	if got := theSportsDBAdvanceMethod("AP"); got != "penalties" {
		t.Fatalf("AP -> %q, want penalties", got)
	}
	if got := theSportsDBAdvanceMethod("FT"); got != "" {
		t.Fatalf("FT -> %q, want empty", got)
	}
}

func TestDeriveKnockoutWinner(t *testing.T) {
	t.Parallel()

	// Vitória regular / AET
	if got := deriveKnockoutWinner(3, 2, nil, nil); got == nil || *got != "home" {
		t.Fatalf("3-2 expected home, got %v", got)
	}
	if got := deriveKnockoutWinner(1, 2, nil, nil); got == nil || *got != "away" {
		t.Fatalf("1-2 expected away, got %v", got)
	}

	// Empate + pênaltis (extras)
	homePen, awayPen := 3, 4
	if got := deriveKnockoutWinner(1, 1, &homePen, &awayPen); got == nil || *got != "away" {
		t.Fatalf("1-1 (3-4 pen) expected away, got %v", got)
	}

	// Empate sem extras → desconhecido
	if got := deriveKnockoutWinner(0, 0, nil, nil); got != nil {
		t.Fatalf("0-0 without extras expected nil, got %v", *got)
	}
}
