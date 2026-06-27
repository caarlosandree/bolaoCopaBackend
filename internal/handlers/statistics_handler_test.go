package handlers

import "testing"

func TestNormalizeRoundMapsTheSportsDBKnockoutNumbers(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"4":       "r32",
		"5":       "r16",
		"6":       "qf",
		"7":       "sf",
		"8":       "third",
		"9":       "final",
		"Round 4": "r32",
		"Round 9": "final",
	}

	for input, want := range tests {
		input := input
		want := want

		t.Run(input, func(t *testing.T) {
			t.Parallel()

			if got := normalizeRound(input); got != want {
				t.Fatalf("normalizeRound(%q) = %q, want %q", input, got, want)
			}
		})
	}
}
