-- Keyword graph nodes.
CREATE TABLE IF NOT EXISTS keywords (
    term             TEXT NOT NULL,
    country          TEXT NOT NULL,
    entity           TEXT NOT NULL,
    parent_term      TEXT,
    parent_country   TEXT,
    parent_entity    TEXT,
    depth            INTEGER NOT NULL,
    source           TEXT NOT NULL,
    discovered_at    TEXT NOT NULL,
    state            TEXT NOT NULL,
    too_broad        INTEGER NOT NULL DEFAULT 0,
    too_broad_reason TEXT NOT NULL DEFAULT '',
    verdict          TEXT NOT NULL DEFAULT 'UNKNOWN',
    cluster_ref      TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (term, country, entity)
);

CREATE INDEX IF NOT EXISTS idx_keywords_state ON keywords(state);
CREATE INDEX IF NOT EXISTS idx_keywords_parent ON keywords(parent_term, parent_country, parent_entity);

-- One current observation per keyword and window.
CREATE TABLE IF NOT EXISTS observations (
    term     TEXT NOT NULL,
    country  TEXT NOT NULL,
    entity   TEXT NOT NULL,
    window   TEXT NOT NULL,
    start_at TEXT NOT NULL,
    end_at   TEXT NOT NULL,
    samples  TEXT NOT NULL,
    PRIMARY KEY (term, country, entity, window),
    FOREIGN KEY (term, country, entity) REFERENCES keywords(term, country, entity) ON DELETE CASCADE
);
