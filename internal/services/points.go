package services

// KnockoutBonus defines bonus points for knockout match tiebreakers.
const (
	bonusAdvanceTeam   = 2
	bonusAdvanceMethod = 1
)

// MatchContext carries match info needed for scoring.
type MatchContext struct {
	IsKnockout    bool
	WinnerTeam    *string // "home" or "away" (set when knockout finishes)
	AdvanceMethod *string // "et" or "penalties" (set when 90 min draw)
}

// GuessContext carries guess info needed for scoring.
type GuessContext struct {
	AdvancingTeam *string // "home" or "away" (only on knockout draw guess)
	AdvanceMethod *string // "et" or "penalties" (only on knockout draw guess)
}

// CalculatePoints calcula os pontos obtidos com base no palpite do usuário e no placar real.
//
// Regras de Negócio de Pontuação:
// - Acertar o placar exato (Ex: Palpite 2x1, Jogo 2x1) = 5 pontos
// - Acertar o vencedor e o saldo de gols (Ex: Palpite 3x1, Jogo 2x0) = 3 pontos
// - Acertar apenas o vencedor errando o placar (Ex: Palpite 1x0, Jogo 3x1) = 2 pontos
// - Acertar o empate sem o placar exato (Ex: Palpite 2x2, Jogo 1x1) = 1 ponto
// - Errar completamente o resultado = 0 pontos
func CalculatePoints(homeGuess, awayGuess, homeScore, awayScore int) int {
	return CalculatePointsWithContext(homeGuess, awayGuess, homeScore, awayScore, MatchContext{}, GuessContext{})
}

// CalculatePointsWithContext calcula pontos considerando contexto de mata-mata.
//
// Bônus de Mata-mata (quando isKnockout e o placar de 90 min é empate):
// - Acertar quem avança = +2 pontos
// - Acertar o método (et vs penalties) = +1 ponto
func CalculatePointsWithContext(homeGuess, awayGuess, homeScore, awayScore int, mc MatchContext, gc GuessContext) int {
	isDraw := homeScore == awayScore

	// 1. Acerto do placar exato -> 5 pontos
	if homeGuess == homeScore && awayGuess == awayScore {
		return 5 + knockoutBonus(mc, gc, isDraw)
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

	// 2. Empate não exato -> 1 ponto + bônus de mata-mata
	if realOutcome == 0 {
		return 1 + knockoutBonus(mc, gc, isDraw)
	}

	// 3. Acerto do vencedor e o saldo de gols (mas não o placar exato) -> 3 pontos
	guessGD := homeGuess - awayGuess
	realGD := homeScore - awayScore
	if guessGD == realGD {
		return 3
	}

	// 4. Acerto apenas do vencedor com saldo de gols diferente -> 2 pontos
	return 2
}

// knockoutBonus calcula o bônus de mata-mata para palpites de empate.
// Só se aplica quando a partida é mata-mata, o placar real é empate (90 min),
// e o winner_team já foi definido (admin ou sync).
func knockoutBonus(mc MatchContext, gc GuessContext, isDraw bool) int {
	if !mc.IsKnockout || !isDraw || mc.WinnerTeam == nil {
		return 0
	}

	bonus := 0

	// Bônus por acertar quem avança
	if gc.AdvancingTeam != nil && *gc.AdvancingTeam == *mc.WinnerTeam {
		bonus += bonusAdvanceTeam
	}

	// Bônus por acertar o método (et vs penalties)
	if gc.AdvanceMethod != nil && mc.AdvanceMethod != nil && *gc.AdvanceMethod == *mc.AdvanceMethod {
		bonus += bonusAdvanceMethod
	}

	return bonus
}
