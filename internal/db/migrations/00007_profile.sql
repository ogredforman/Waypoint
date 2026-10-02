-- Adds the profile table (PLAN 1.5.8, ADR 0005): every database file is one
-- profile, and this one row holds its display name. The name lives in the
-- database, not in a registry file, so a profile file describes itself.
-- The row is inserted here as "Personal": that names the database that existed
-- before profiles, and a new profile is renamed before it goes live (it is
-- built under a temporary file name first). Name rules (trimmed, 1 to 60
-- characters, unique ignoring case across profiles) are checked in Go.
-- Times are Unix milliseconds, as elsewhere; profiles are listed by created_at.

-- +goose Up

CREATE TABLE profile (
    id         INTEGER PRIMARY KEY CHECK (id = 1),
    name       TEXT    NOT NULL CHECK (length(trim(name)) > 0),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO profile (id, name, created_at, updated_at)
VALUES (1, 'Personal',
        CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER),
        CAST((julianday('now') - 2440587.5) * 86400000 AS INTEGER));

-- +goose Down

DROP TABLE profile;
