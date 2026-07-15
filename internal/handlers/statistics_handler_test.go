package handlers

import (
	"strings"
	"testing"
)

func TestNormalizeRoundMapsTheSportsDBKnockoutNumbers(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"4":       "r32",
		"5":       "r16",
		"6":       "qf",
		"7":       "sf",
		"8":       "third",
		"9":       "final",
		"32":      "r32",
		"16":      "r16",
		"125":     "qf",
		"150":     "sf",
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

func TestOrderPreviousRoundByNextPairsSources(t *testing.T) {
	t.Parallel()

	// R32 em ordem "cronológica" errada para o desenho do bracket
	prev := []bracketMatchDTO{
		{ID: "a", Home: bracketTeamDTO{Name: "South Africa"}, Away: bracketTeamDTO{Name: "Canada"}},
		{ID: "b", Home: bracketTeamDTO{Name: "Brazil"}, Away: bracketTeamDTO{Name: "Japan"}},
		{ID: "c", Home: bracketTeamDTO{Name: "Netherlands"}, Away: bracketTeamDTO{Name: "Morocco"}},
		{ID: "d", Home: bracketTeamDTO{Name: "Ivory Coast"}, Away: bracketTeamDTO{Name: "Norway"}},
	}
	// Oitavas: Canada×Morocco e Brazil×Norway → pares (a,c) e (b,d)
	next := []bracketMatchDTO{
		{ID: "r16-0", Home: bracketTeamDTO{Name: "Canada"}, Away: bracketTeamDTO{Name: "Morocco"}},
		{ID: "r16-1", Home: bracketTeamDTO{Name: "Brazil"}, Away: bracketTeamDTO{Name: "Norway"}},
	}

	got := orderPreviousRoundByNext(prev, next)
	if len(got) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(got))
	}
	wantIDs := []string{"a", "c", "b", "d"}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("slot %d: got %s, want %s (full=%v)", i, got[i].ID, id, idsOf(got))
		}
	}
}

func TestOrderKnockoutBracketAssignsSlots(t *testing.T) {
	t.Parallel()

	rounds := map[string][]bracketMatchDTO{
		"r32": {
			{ID: "m1", Home: bracketTeamDTO{Name: "A"}, Away: bracketTeamDTO{Name: "B"}, MatchTime: "2026-06-28T19:00:00"},
			{ID: "m2", Home: bracketTeamDTO{Name: "C"}, Away: bracketTeamDTO{Name: "D"}, MatchTime: "2026-06-29T17:00:00"},
			{ID: "m3", Home: bracketTeamDTO{Name: "E"}, Away: bracketTeamDTO{Name: "F"}, MatchTime: "2026-06-29T20:30:00"},
			{ID: "m4", Home: bracketTeamDTO{Name: "G"}, Away: bracketTeamDTO{Name: "H"}, MatchTime: "2026-06-30T01:00:00"},
		},
		"r16": {
			{ID: "n1", Home: bracketTeamDTO{Name: "B"}, Away: bracketTeamDTO{Name: "F"}, MatchTime: "2026-07-04T17:00:00"},
			{ID: "n2", Home: bracketTeamDTO{Name: "D"}, Away: bracketTeamDTO{Name: "H"}, MatchTime: "2026-07-04T21:00:00"},
		},
		"qf":    {},
		"sf":    {},
		"final": {},
		"third": {},
	}

	orderKnockoutBracket(rounds)

	// B veio de m1, F de m3 → slots 0,1; D de m2, H de m4 → slots 2,3
	got := rounds["r32"]
	wantIDs := []string{"m1", "m3", "m2", "m4"}
	for i, id := range wantIDs {
		if got[i].ID != id || got[i].Slot != i {
			t.Fatalf("r32[%d]=%s slot=%d, want %s slot=%d", i, got[i].ID, got[i].Slot, id, i)
		}
	}
	for i := range rounds["r16"] {
		if rounds["r16"][i].Slot != i {
			t.Fatalf("r16 slot %d != %d", rounds["r16"][i].Slot, i)
		}
	}
}

func idsOf(matches []bracketMatchDTO) []string {
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = m.ID
	}
	return out
}

func TestFillFinalAndThirdFromSemis(t *testing.T) {
	t.Parallel()

	rounds := map[string][]bracketMatchDTO{
		"sf": {
			{
				ID:     "sf1",
				Home:   bracketTeamDTO{Name: "France", Score: intPtr(0)},
				Away:   bracketTeamDTO{Name: "Spain", Score: intPtr(2)},
				Status: "finished",
			},
			{
				ID:   "sf2",
				Home: bracketTeamDTO{Name: "England"},
				Away: bracketTeamDTO{Name: "Argentina"},
				// em andamento — sem vencedor
				Status: "ongoing",
			},
		},
		"final": {},
		"third": {},
	}

	fillFinalAndThirdFromSemis(rounds)

	if len(rounds["final"]) != 1 {
		t.Fatalf("expected synthetic final, got %d", len(rounds["final"]))
	}
	final := rounds["final"][0]
	if final.Home.Name != "Spain" {
		t.Fatalf("final home=%q, want Spain", final.Home.Name)
	}
	if !stringsContainsFold(final.Away.Name, "England") || !stringsContainsFold(final.Away.Name, "Argentina") {
		t.Fatalf("final away placeholder=%q, want Vencedor England/Argentina", final.Away.Name)
	}
	// 3º só com duas semis finalizadas
	if len(rounds["third"]) != 0 {
		t.Fatalf("third should be empty until both semis finish, got %v", rounds["third"])
	}

	// Completa a segunda semi
	awayW := "away"
	rounds["sf"][1].Status = "finished"
	rounds["sf"][1].Home.Score = intPtr(1)
	rounds["sf"][1].Away.Score = intPtr(2)
	rounds["sf"][1].WinnerTeam = &awayW
	fillFinalAndThirdFromSemis(rounds)

	if rounds["final"][0].Away.Name != "Argentina" {
		t.Fatalf("final away=%q, want Argentina", rounds["final"][0].Away.Name)
	}
	if len(rounds["third"]) != 1 {
		t.Fatalf("expected third place match, got %d", len(rounds["third"]))
	}
	third := rounds["third"][0]
	// France perdeu sf1; England perdeu sf2
	names := []string{third.Home.Name, third.Away.Name}
	if !containsAll(names, "France", "England") {
		t.Fatalf("third teams=%v, want France and England", names)
	}
}

func TestClassifyKnockoutBySemisFinal(t *testing.T) {
	t.Parallel()

	semis := []bracketMatchDTO{
		{
			ID: "sf1", Home: bracketTeamDTO{Name: "France", Score: intPtr(0)},
			Away: bracketTeamDTO{Name: "Spain", Score: intPtr(2)}, Status: "finished",
		},
		{
			ID: "sf2", Home: bracketTeamDTO{Name: "England", Score: intPtr(1)},
			Away: bracketTeamDTO{Name: "Argentina", Score: intPtr(2)}, Status: "finished",
		},
	}
	final := bracketMatchDTO{
		ID: "f1", Home: bracketTeamDTO{Name: "Spain"}, Away: bracketTeamDTO{Name: "Argentina"},
	}
	if got := classifyKnockoutBySemis(final, semis); got != "final" {
		t.Fatalf("classify final = %q", got)
	}
	third := bracketMatchDTO{
		ID: "t1", Home: bracketTeamDTO{Name: "France"}, Away: bracketTeamDTO{Name: "England"},
	}
	if got := classifyKnockoutBySemis(third, semis); got != "third" {
		t.Fatalf("classify third = %q", got)
	}
}

func intPtr(v int) *int { return &v }

func stringsContainsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func containsAll(have []string, want ...string) bool {
	set := map[string]bool{}
	for _, h := range have {
		set[normalizeTeamName(h)] = true
	}
	for _, w := range want {
		if !set[normalizeTeamName(w)] {
			return false
		}
	}
	return true
}
