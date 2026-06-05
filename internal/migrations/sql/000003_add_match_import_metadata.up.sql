ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS external_source TEXT,
    ADD COLUMN IF NOT EXISTS external_id TEXT,
    ADD COLUMN IF NOT EXISTS worldcup26_match_id TEXT,
    ADD COLUMN IF NOT EXISTS group_name VARCHAR(50),
    ADD COLUMN IF NOT EXISTS venue VARCHAR(120),
    ADD COLUMN IF NOT EXISTS match_number INT,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tournaments_name
    ON tournaments(name);

CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_external_source_id
    ON matches(external_source, external_id)
    WHERE external_source IS NOT NULL AND external_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_worldcup26_match_id
    ON matches(worldcup26_match_id)
    WHERE worldcup26_match_id IS NOT NULL;
