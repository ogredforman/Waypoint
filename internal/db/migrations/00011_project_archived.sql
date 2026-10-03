-- Adds the archived project status (PLAN 1.5.12, ADR 0006): a project kept for
-- the record but hidden from the Now screen, lists, pickers, and the
-- extraction snapshot. SQLite can't alter a CHECK constraint, so the table is
-- rebuilt.
--
-- Most tables reference projects, so the old rows are copied aside, the table
-- dropped and recreated under its own name, and the rows copied back. Foreign
-- key checks are deferred to commit (this transaction only); by then every
-- reference has its project again. The columns keep their order, since the
-- generated queries select *. The CHECK list must match
-- domain.ProjectStatuses(), values and order.

-- +goose Up

PRAGMA defer_foreign_keys = ON;

CREATE TABLE projects_copy (
    id                 INTEGER PRIMARY KEY,
    title              TEXT    NOT NULL,
    why                TEXT    NOT NULL,
    definition_of_done TEXT    NOT NULL,
    status             TEXT    NOT NULL,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
INSERT INTO projects_copy
SELECT id, title, why, definition_of_done, status, created_at, updated_at
FROM projects;

DROP TABLE projects;

CREATE TABLE projects (
    id                 INTEGER PRIMARY KEY,
    title              TEXT    NOT NULL,
    why                TEXT    NOT NULL DEFAULT '',
    definition_of_done TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL CHECK (status IN ('active', 'paused', 'parked', 'done', 'archived')),
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);

INSERT INTO projects (id, title, why, definition_of_done, status, created_at, updated_at)
SELECT id, title, why, definition_of_done, status, created_at, updated_at
FROM projects_copy;

DROP TABLE projects_copy;

CREATE INDEX projects_status_idx ON projects (status);

-- +goose Down

-- The old table has no archived status. Archived projects become parked,
-- which is where restoring one puts it.
PRAGMA defer_foreign_keys = ON;

UPDATE projects SET status = 'parked' WHERE status = 'archived';

CREATE TABLE projects_copy (
    id                 INTEGER PRIMARY KEY,
    title              TEXT    NOT NULL,
    why                TEXT    NOT NULL,
    definition_of_done TEXT    NOT NULL,
    status             TEXT    NOT NULL,
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
INSERT INTO projects_copy
SELECT id, title, why, definition_of_done, status, created_at, updated_at
FROM projects;

DROP TABLE projects;

CREATE TABLE projects (
    id                 INTEGER PRIMARY KEY,
    title              TEXT    NOT NULL,
    why                TEXT    NOT NULL DEFAULT '',
    definition_of_done TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL CHECK (status IN ('active', 'paused', 'parked', 'done')),
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);

INSERT INTO projects (id, title, why, definition_of_done, status, created_at, updated_at)
SELECT id, title, why, definition_of_done, status, created_at, updated_at
FROM projects_copy;

DROP TABLE projects_copy;

CREATE INDEX projects_status_idx ON projects (status);
