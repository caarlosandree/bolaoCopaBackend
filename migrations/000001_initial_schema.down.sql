-- Rollback da Migration Inicial: Remoção do Schema do Banco de Dados

-- Remove tabelas em ordem reversa (devido a foreign keys)
DROP INDEX IF EXISTS idx_guesses_user_id;
DROP INDEX IF EXISTS idx_guesses_match_id;
DROP TABLE IF EXISTS guesses;

DROP INDEX IF EXISTS idx_matches_round_id;
DROP INDEX IF EXISTS idx_matches_match_time;
DROP TABLE IF EXISTS matches;

DROP TABLE IF EXISTS rounds;

DROP TABLE IF EXISTS tournaments;

DROP INDEX IF EXISTS idx_users_total_points;
DROP TABLE IF EXISTS users;

DROP EXTENSION IF EXISTS "uuid-ossp";
