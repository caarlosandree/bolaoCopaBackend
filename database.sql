-- Script SQL Inicial (DDL) para o Banco de Dados PostgreSQL
-- Projeto: Bolão da Copa do Mundo - Corporativo

-- Extensões úteis (para UUIDs se necessário, embora usemos SERIAL por simplicidade neste MVP)
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Tabela de Usuários (users)
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(20) DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    total_points INT DEFAULT 0 CHECK (total_points >= 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index para otimização do ranking
CREATE INDEX IF NOT EXISTS idx_users_total_points ON users(total_points DESC);

-- 2. Tabela de Torneios (tournaments)
CREATE TABLE IF NOT EXISTS tournaments (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    active BOOLEAN DEFAULT TRUE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tournaments_name ON tournaments(name);

-- 3. Tabela de Rodadas (rounds)
CREATE TABLE IF NOT EXISTS rounds (
    id SERIAL PRIMARY KEY,
    tournament_id INT REFERENCES tournaments(id) ON DELETE CASCADE,
    number INT NOT NULL,
    name VARCHAR(50) NOT NULL,
    status VARCHAR(20) DEFAULT 'upcoming' CHECK (status IN ('upcoming', 'active', 'finished')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_round_number_tournament UNIQUE (tournament_id, number)
);

-- 4. Tabela de Partidas (matches)
CREATE TABLE IF NOT EXISTS matches (
    id SERIAL PRIMARY KEY,
    round_id INT REFERENCES rounds(id) ON DELETE CASCADE,
    home_team VARCHAR(100) NOT NULL,
    away_team VARCHAR(100) NOT NULL,
    home_score INT DEFAULT NULL,
    away_score INT DEFAULT NULL,
    status VARCHAR(20) DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'ongoing', 'finished')),
    match_time TIMESTAMP WITH TIME ZONE NOT NULL,
    external_source TEXT,
    external_id TEXT,
    worldcup26_match_id TEXT,
    group_name VARCHAR(50),
    venue VARCHAR(120),
    match_number INT,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Index para buscar partidas por rodada e horário (essencial para validação de bloqueio)
CREATE INDEX IF NOT EXISTS idx_matches_round_id ON matches(round_id);
CREATE INDEX IF NOT EXISTS idx_matches_match_time ON matches(match_time);
CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_external_source_id
    ON matches(external_source, external_id)
    WHERE external_source IS NOT NULL AND external_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_worldcup26_match_id
    ON matches(worldcup26_match_id)
    WHERE worldcup26_match_id IS NOT NULL;

-- 5. Tabela de Palpites (guesses)
CREATE TABLE IF NOT EXISTS guesses (
    id SERIAL PRIMARY KEY,
    user_id INT REFERENCES users(id) ON DELETE CASCADE,
    match_id INT REFERENCES matches(id) ON DELETE CASCADE,
    home_guess INT NOT NULL CHECK (home_guess >= 0),
    away_guess INT NOT NULL CHECK (away_guess >= 0),
    points_earned INT DEFAULT NULL CHECK (points_earned >= 0),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    -- Garante que o usuário só tenha um palpite por partida
    CONSTRAINT unique_user_match_guess UNIQUE (user_id, match_id)
);

-- Index para agilizar a consulta de palpites por usuário
CREATE INDEX IF NOT EXISTS idx_guesses_user_id ON guesses(user_id);
CREATE INDEX IF NOT EXISTS idx_guesses_match_id ON guesses(match_id);

-- 6. Tabela de Auditoria (audit_logs)
CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    request_id TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    actor_user_id INTEGER NULL REFERENCES users(id),
    actor_role TEXT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NULL,
    resource_id TEXT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status_code INTEGER NOT NULL,
    outcome TEXT NOT NULL,
    ip INET NULL,
    user_agent TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_request_id ON audit_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_actor_time ON audit_logs(actor_user_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_time ON audit_logs(occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action_time ON audit_logs(action, occurred_at DESC);
