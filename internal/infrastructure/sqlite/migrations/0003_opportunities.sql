-- One business decision per keyword (BR-20).
CREATE TABLE IF NOT EXISTS opportunities (
    term       TEXT NOT NULL,
    country    TEXT NOT NULL,
    entity     TEXT NOT NULL,
    intent     TEXT NOT NULL,
    trend      TEXT NOT NULL,
    state      TEXT NOT NULL,
    note       TEXT NOT NULL DEFAULT '',
    evidence   TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (term, country, entity)
);

CREATE INDEX IF NOT EXISTS idx_opportunities_state ON opportunities(state);
