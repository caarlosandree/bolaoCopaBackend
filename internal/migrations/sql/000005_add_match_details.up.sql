ALTER TABLE matches
    ADD COLUMN IF NOT EXISTS thesportsdb_event_id TEXT,
    ADD COLUMN IF NOT EXISTS thesportsdb_home_team_id TEXT,
    ADD COLUMN IF NOT EXISTS thesportsdb_away_team_id TEXT,
    ADD COLUMN IF NOT EXISTS odds_api_event_id TEXT,
    ADD COLUMN IF NOT EXISTS api_football_fixture_id TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_thesportsdb_event_id
    ON matches(thesportsdb_event_id)
    WHERE thesportsdb_event_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_matches_odds_api_event_id
    ON matches(odds_api_event_id)
    WHERE odds_api_event_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS match_details (
    match_id INT PRIMARY KEY REFERENCES matches(id) ON DELETE CASCADE,
    odds JSONB,
    predictions JSONB,
    recent_form JSONB,
    head_to_head JSONB,
    lineups JSONB,
    statistics JSONB,
    injuries JSONB,
    events JSONB,
    media JSONB,
    source_status JSONB,
    last_synced_at TIMESTAMPTZ,
    lineups_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
