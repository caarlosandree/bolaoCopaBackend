package services

// CalculatePoints calcula os pontos obtidos com base no palpite do usuário e no placar real.
//
// Regras de Negócio de Pontuação:
// - Acertar o placar exato (Ex: Palpite 2x1, Jogo 2x1) = 5 pontos
// - Acertar o vencedor e o saldo de gols (Ex: Palpite 3x1, Jogo 2x0) = 3 pontos
// - Acertar apenas o vencedor ou empate errando o placar (Ex: Palpite 1x0, Jogo 3x1) = 2 pontos
// - Errar completamente o resultado = 0 pontos
func CalculatePoints(homeGuess, awayGuess, homeScore, awayScore int) int {
	// 1. Acerto do placar exato -> 5 pontos
	if homeGuess == homeScore && awayGuess == awayScore {
		return 5
	}

	// Classificação dos resultados:
	//  1: Vitória do time mandante (Home Win)
	// -1: Vitória do time visitante (Away Win)
	//  0: Empate (Draw)
	var guessOutcome int
	if homeGuess > awayGuess {
		guessOutcome = 1
	} else if homeGuess < awayGuess {
		guessOutcome = -1
	} else {
		guessOutcome = 0
	}

	var realOutcome int
	if homeScore > awayScore {
		realOutcome = 1
	} else if homeScore < awayScore {
		realOutcome = -1
	} else {
		realOutcome = 0
	}

	// Se errou o vencedor / empate -> 0 pontos
	if guessOutcome != realOutcome {
		return 0
	}

	// 2. Acerto do vencedor e o saldo de gols (mas não o placar exato) -> 3 pontos
	// O saldo é calculado como (gols do mandante - gols do visitante)
	guessGD := homeGuess - awayGuess
	realGD := homeScore - awayScore
	if guessGD == realGD {
		return 3
	}

	// 3. Acerto apenas do vencedor/empate com saldo de gols diferente -> 2 pontos
	return 2
}
