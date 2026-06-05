package models

import "time"

// User representa a entidade de usuário no banco de dados.
type User struct {
	ID           int       `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	Email        string    `json:"email" db:"email"`
	PasswordHash string    `json:"-" db:"password_hash"` // Oculto no JSON para segurança
	Role         string    `json:"role" db:"role"`       // "admin" ou "user"
	TotalPoints  int       `json:"total_points" db:"total_points"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Tournament representa um campeonato/torneio (ex: Copa do Mundo 2026).
type Tournament struct {
	ID     int    `json:"id" db:"id"`
	Name   string `json:"name" db:"name"`
	Active bool   `json:"active" db:"active"`
}

// Round representa as rodadas do bolão (ex: Rodada 1 - Fase de Grupos).
type Round struct {
	ID           int       `json:"id" db:"id"`
	TournamentID int       `json:"tournament_id" db:"tournament_id"`
	Number       int       `json:"number" db:"number"`
	Name         string    `json:"name" db:"name"`
	Status       string    `json:"status" db:"status"` // "upcoming", "active", "finished"
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Match representa uma partida real do campeonato.
type Match struct {
	ID        int        `json:"id" db:"id"`
	RoundID   int        `json:"round_id" db:"round_id"`
	HomeTeam  string     `json:"home_team" db:"home_team"`
	AwayTeam  string     `json:"away_team" db:"away_team"`
	HomeScore *int       `json:"home_score" db:"home_score"` // Ponteiro para permitir null (não finalizado)
	AwayScore *int       `json:"away_score" db:"away_score"` // Ponteiro para permitir null (não finalizado)
	Status    string     `json:"status" db:"status"`         // "scheduled", "ongoing", "finished"
	MatchTime time.Time  `json:"match_time" db:"match_time"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UserGuess *UserGuess `json:"user_guess,omitempty"`       // Vinculado dinamicamente para exibição
}

// UserGuess representa dados simplificados do palpite do usuário para a partida.
type UserGuess struct {
	ID           int  `json:"id,omitempty"`
	HomeGuess    int  `json:"home_guess"`
	AwayGuess    int  `json:"away_guess"`
	PointsEarned *int `json:"points_earned,omitempty"`
}

// Guess representa a tabela de palpites completa.
type Guess struct {
	ID           int       `json:"id" db:"id"`
	UserID       int       `json:"user_id" db:"user_id"`
	MatchID      int       `json:"match_id" db:"match_id"`
	HomeGuess    int       `json:"home_guess" db:"home_guess"`
	AwayGuess    int       `json:"away_guess" db:"away_guess"`
	PointsEarned *int      `json:"points_earned" db:"points_earned"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// Ranking representa uma entrada de linha na tabela de classificação geral.
type Ranking struct {
	Position    int    `json:"position"`
	UserID      int    `json:"user_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	TotalPoints int    `json:"total_points"`
}
