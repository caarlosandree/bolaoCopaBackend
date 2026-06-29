-- Adiciona suporte a mata-mata: empate, prorrogação e pênaltis.
--
-- matches:
--   is_knockout    -> indica fase eliminatória (empate abre opções extras no palpite)
--   winner_team     -> quem avançou ("home" ou "away"), preenchido ao finalizar
--   advance_method  -> como foi decidido após empate ("et" = prorrogação, "penalties" = pênaltis)
--
-- guesses:
--   advancing_team  -> quem o usuário acha que avança ("home" ou "away"), só em empate de mata-mata
--   advance_method  -> como o usuário acha que será decidido ("et" ou "penalties")

ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS is_knockout BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS winner_team VARCHAR(10) DEFAULT NULL
        CHECK (winner_team IS NULL OR winner_team IN ('home', 'away')),
    ADD COLUMN IF NOT EXISTS advance_method VARCHAR(10) DEFAULT NULL
        CHECK (advance_method IS NULL OR advance_method IN ('et', 'penalties'));

ALTER TABLE guesses
    ADD COLUMN IF NOT EXISTS advancing_team VARCHAR(10) DEFAULT NULL
        CHECK (advancing_team IS NULL OR advancing_team IN ('home', 'away')),
    ADD COLUMN IF NOT EXISTS advance_method VARCHAR(10) DEFAULT NULL
        CHECK (advance_method IS NULL OR advance_method IN ('et', 'penalties'));
