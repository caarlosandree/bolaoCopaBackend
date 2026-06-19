-- Adiciona flag de ocultação de usuário para o ranking público
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_hidden BOOLEAN NOT NULL DEFAULT FALSE;

-- Índice para facilitar filtros de usuários visíveis no ranking
CREATE INDEX IF NOT EXISTS idx_users_is_hidden ON users(is_hidden);
