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
