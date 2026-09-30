-- Lets a changeset be split when it is applied (ADR 0003): ops left undecided,
-- or held back by the active-project cap, move to a new pending changeset whose
-- parent_changeset_id points at the one they came from. The child keeps the
-- parent's extraction run, so extraction_run_id is no longer unique.
--
-- SQLite can't drop a UNIQUE constraint, so the table is rebuilt. change_ops
-- references changesets, so the old rows are copied aside, the table dropped
-- and recreated under its own name, and the rows copied back. Foreign key
-- checks are deferred to commit (this transaction only); by then every op has
-- its changeset again.

-- +goose Up

PRAGMA defer_foreign_keys = ON;

CREATE TABLE changesets_copy (
    id                INTEGER PRIMARY KEY,
    source            TEXT    NOT NULL,
    extraction_run_id INTEGER,
    status            TEXT    NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    resolved_at       INTEGER
);
INSERT INTO changesets_copy
SELECT id, source, extraction_run_id, status, created_at, updated_at, resolved_at
FROM changesets;

DROP TABLE changesets;

-- An extraction changeset has exactly one run; a manual one has none. A
-- changeset split off another has the same source and run as its parent.
CREATE TABLE changesets (
    id                  INTEGER PRIMARY KEY,
    source              TEXT    NOT NULL CHECK (source IN ('extraction', 'manual')),
    extraction_run_id   INTEGER REFERENCES extraction_runs (id),
    parent_changeset_id INTEGER REFERENCES changesets (id),
    status              TEXT    NOT NULL CHECK (status IN ('pending', 'applied', 'partially_applied', 'rejected')),
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL,
    resolved_at         INTEGER,
    CHECK ((source = 'extraction') = (extraction_run_id IS NOT NULL))
);

INSERT INTO changesets (id, source, extraction_run_id, status, created_at, updated_at, resolved_at)
SELECT id, source, extraction_run_id, status, created_at, updated_at, resolved_at
FROM changesets_copy;

DROP TABLE changesets_copy;

CREATE INDEX changesets_status_idx ON changesets (status);
CREATE INDEX changesets_extraction_run_id_idx ON changesets (extraction_run_id);
CREATE INDEX changesets_parent_changeset_id_idx ON changesets (parent_changeset_id);

-- +goose Down

-- The old table allows one changeset per run, so changesets split off another
-- are dropped along with their ops.
PRAGMA defer_foreign_keys = ON;

DELETE FROM change_ops
WHERE changeset_id IN (SELECT id FROM changesets WHERE parent_changeset_id IS NOT NULL);
DELETE FROM changesets WHERE parent_changeset_id IS NOT NULL;

CREATE TABLE changesets_copy (
    id                INTEGER PRIMARY KEY,
    source            TEXT    NOT NULL,
    extraction_run_id INTEGER,
    status            TEXT    NOT NULL,
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    resolved_at       INTEGER
);
INSERT INTO changesets_copy
SELECT id, source, extraction_run_id, status, created_at, updated_at, resolved_at
FROM changesets;

DROP TABLE changesets;

CREATE TABLE changesets (
    id                INTEGER PRIMARY KEY,
    source            TEXT    NOT NULL CHECK (source IN ('extraction', 'manual')),
    extraction_run_id INTEGER UNIQUE REFERENCES extraction_runs (id),
    status            TEXT    NOT NULL CHECK (status IN ('pending', 'applied', 'partially_applied', 'rejected')),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL,
    resolved_at       INTEGER,
    CHECK ((source = 'extraction') = (extraction_run_id IS NOT NULL))
);

INSERT INTO changesets (id, source, extraction_run_id, status, created_at, updated_at, resolved_at)
SELECT id, source, extraction_run_id, status, created_at, updated_at, resolved_at
FROM changesets_copy;

DROP TABLE changesets_copy;

CREATE INDEX changesets_status_idx ON changesets (status);
