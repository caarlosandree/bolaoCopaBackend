ALTER TABLE matches
    DROP COLUMN IF EXISTS advance_method,
    DROP COLUMN IF EXISTS winner_team,
    DROP COLUMN IF EXISTS is_knockout;

ALTER TABLE guesses
    DROP COLUMN IF EXISTS advance_method,
    DROP COLUMN IF EXISTS advancing_team;
