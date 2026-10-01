-- Adds the plan observation kind (PLAN 1.5.4): a line saying what the user
-- means to do, such as the mock's "todo:" lines. SQLite can't alter a CHECK
-- constraint, so the table is rebuilt.
--
-- observation_links references observations, so the old rows are copied aside,
-- the table dropped and recreated under its own name, and the rows copied back.
-- Foreign key checks are deferred to commit (this transaction only); by then
-- every link has its observation again. The columns keep their order, since
-- the generated queries select *. The CHECK list must match
-- domain.ObservationKinds(), values and order.

-- +goose Up

PRAGMA defer_foreign_keys = ON;

CREATE TABLE observations_copy (
    id                INTEGER PRIMARY KEY,
    entry_id          INTEGER NOT NULL,
    extraction_run_id INTEGER NOT NULL,
    kind              TEXT    NOT NULL,
    summary           TEXT    NOT NULL,
    span_start        INTEGER NOT NULL,
    span_end          INTEGER NOT NULL,
    created_at        INTEGER NOT NULL
);
INSERT INTO observations_copy
SELECT id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
FROM observations;

DROP TABLE observations;

CREATE TABLE observations (
    id                INTEGER PRIMARY KEY,
    entry_id          INTEGER NOT NULL REFERENCES entries (id),
    extraction_run_id INTEGER NOT NULL REFERENCES extraction_runs (id),
    kind              TEXT    NOT NULL CHECK (kind IN ('progress', 'plan', 'blocker', 'idea', 'feeling', 'decision', 'question', 'note')),
    summary           TEXT    NOT NULL,
    span_start        INTEGER NOT NULL CHECK (span_start >= 0),
    span_end          INTEGER NOT NULL,
    created_at        INTEGER NOT NULL,
    CHECK (span_end >= span_start)
);

INSERT INTO observations (id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at)
SELECT id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
FROM observations_copy;

DROP TABLE observations_copy;

CREATE INDEX observations_entry_id_idx ON observations (entry_id);
CREATE INDEX observations_extraction_run_id_idx ON observations (extraction_run_id);

-- +goose Down

-- The old table has no plan kind. Plan observations become notes, keeping
-- their text, span, and links.
PRAGMA defer_foreign_keys = ON;

UPDATE observations SET kind = 'note' WHERE kind = 'plan';

CREATE TABLE observations_copy (
    id                INTEGER PRIMARY KEY,
    entry_id          INTEGER NOT NULL,
    extraction_run_id INTEGER NOT NULL,
    kind              TEXT    NOT NULL,
    summary           TEXT    NOT NULL,
    span_start        INTEGER NOT NULL,
    span_end          INTEGER NOT NULL,
    created_at        INTEGER NOT NULL
);
INSERT INTO observations_copy
SELECT id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
FROM observations;

DROP TABLE observations;

CREATE TABLE observations (
    id                INTEGER PRIMARY KEY,
    entry_id          INTEGER NOT NULL REFERENCES entries (id),
    extraction_run_id INTEGER NOT NULL REFERENCES extraction_runs (id),
    kind              TEXT    NOT NULL CHECK (kind IN ('progress', 'blocker', 'idea', 'feeling', 'decision', 'question', 'note')),
    summary           TEXT    NOT NULL,
    span_start        INTEGER NOT NULL CHECK (span_start >= 0),
    span_end          INTEGER NOT NULL,
    created_at        INTEGER NOT NULL,
    CHECK (span_end >= span_start)
);

INSERT INTO observations (id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at)
SELECT id, entry_id, extraction_run_id, kind, summary, span_start, span_end, created_at
FROM observations_copy;

DROP TABLE observations_copy;

CREATE INDEX observations_entry_id_idx ON observations (entry_id);
CREATE INDEX observations_extraction_run_id_idx ON observations (extraction_run_id);
