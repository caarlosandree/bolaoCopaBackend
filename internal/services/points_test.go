package services

import "testing"

func TestCalculatePoints(t *testing.T) {
	tests := []struct {
		name       string
		homeGuess  int
		awayGuess  int
		homeScore  int
		awayScore  int
		wantPoints int
	}{
		// Exato -> 5
		{"exato home win", 2, 1, 2, 1, 5},
		{"exato away win", 1, 3, 1, 3, 5},
		{"exato draw", 1, 1, 1, 1, 5},
		{"exato 0x0", 0, 0, 0, 0, 5},

		// Vencedor + saldo -> 3
		{"winner and gd home", 3, 1, 2, 0, 3},
		{"winner and gd away", 0, 2, 1, 3, 3},

		// Vencedor apenas -> 2
		{"winner only home", 1, 0, 3, 1, 2},
		{"winner only away", 0, 1, 1, 3, 2},

		// Empate não exato -> 1
		{"draw non-exact 2x2 vs 1x1", 2, 2, 1, 1, 1},
		{"draw non-exact 0x0 vs 1x1", 0, 0, 1, 1, 1},
		{"draw non-exact 5x5 vs 2x2", 5, 5, 2, 2, 1},

		// Errado -> 0
		{"wrong home vs draw", 2, 1, 1, 1, 0},
		{"wrong draw vs home", 1, 1, 2, 1, 0},
		{"wrong home vs away", 2, 1, 1, 3, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePoints(tt.homeGuess, tt.awayGuess, tt.homeScore, tt.awayScore)
			if got != tt.wantPoints {
				t.Errorf("CalculatePoints(%d, %d, %d, %d) = %d; want %d",
					tt.homeGuess, tt.awayGuess, tt.homeScore, tt.awayScore, got, tt.wantPoints)
			}
		})
	}
}

func TestCalculatePointsWithContext(t *testing.T) {
	home := "home"
	away := "away"
	et := "et"
	penalties := "penalties"

	tests := []struct {
		name       string
		homeGuess  int
		awayGuess  int
		homeScore  int
		awayScore  int
		mc         MatchContext
		gc         GuessContext
		wantPoints int
	}{
		// Vitória em mata-mata: sem bônus (não há empate)
		{"knockout home win exact", 2, 1, 2, 1,
			MatchContext{IsKnockout: true}, GuessContext{}, 5},
		{"knockout away win exact", 0, 1, 0, 1,
			MatchContext{IsKnockout: true}, GuessContext{}, 5},

		// Empate exato em mata-mata + bônus total (acertou quem avança + método)
		{"knockout draw exact + both bonuses", 1, 1, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &et}, 8},
		{"knockout draw exact + both bonuses penalties", 2, 2, 2, 2,
			MatchContext{IsKnockout: true, WinnerTeam: &away, AdvanceMethod: &penalties},
			GuessContext{AdvancingTeam: &away, AdvanceMethod: &penalties}, 8},

		// Empate exato em mata-mata + só acertou quem avança
		{"knockout draw exact + advance team only", 1, 1, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &penalties}, 7},

		// Empate exato em mata-mata + só acertou método
		{"knockout draw exact + method only", 1, 1, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &away, AdvanceMethod: &et}, 6},

		// Empate exato em mata-mata + errou ambos
		{"knockout draw exact + no bonus", 1, 1, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &away, AdvanceMethod: &penalties}, 5},

		// Empate não exato em mata-mata + bônus total
		{"knockout draw non-exact + both bonuses", 2, 2, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &penalties},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &penalties}, 4},

		// Empate não exato em mata-mata + só acertou quem avança
		{"knockout draw non-exact + advance team only", 3, 3, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &away, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &away, AdvanceMethod: &penalties}, 3},

		// Empate não exato em mata-mata + errou ambos
		{"knockout draw non-exact + no bonus", 0, 0, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home, AdvanceMethod: &et},
			GuessContext{AdvancingTeam: &away, AdvanceMethod: &penalties}, 1},

		// Empate em mata-mata mas winner_team ainda não definido (admin não preencheu)
		{"knockout draw exact no winner yet", 1, 1, 1, 1,
			MatchContext{IsKnockout: true, WinnerTeam: nil},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &et}, 5},

		// Empate em fase de grupos (não é mata-mata): sem bônus
		{"group stage draw exact no bonus", 1, 1, 1, 1,
			MatchContext{IsKnockout: false},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &et}, 5},

		// Vitória em mata-mata com advancing_team preenchido (deve ser ignorado)
		{"knockout home win with advancing team ignored", 2, 1, 2, 1,
			MatchContext{IsKnockout: true, WinnerTeam: &home},
			GuessContext{AdvancingTeam: &home, AdvanceMethod: &et}, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculatePointsWithContext(tt.homeGuess, tt.awayGuess, tt.homeScore, tt.awayScore, tt.mc, tt.gc)
			if got != tt.wantPoints {
				t.Errorf("CalculatePointsWithContext() = %d; want %d", got, tt.wantPoints)
			}
		})
	}
}
