-- Adds the create_milestone, update_milestone, and delete_milestone op types
-- (PLAN 1.5.18). SQLite can't alter a CHECK constraint, so change_ops is
-- rebuilt the same way as in 00012: the rows are copied aside, the table
-- dropped and recreated under its own name, and the rows copied back, with
-- foreign key checks deferred to commit so the pending links that reference
-- change_ops find their ops again. The columns keep their order, since the
-- generated queries select *. The CHECK list must match domain.OpTypes(),
-- values and order.

-- +goose Up

PRAGMA defer_foreign_keys = ON;

CREATE TABLE change_ops_copy (
    id              INTEGER PRIMARY KEY,
    changeset_id    INTEGER NOT NULL,
    position        INTEGER NOT NULL,
    op_type         TEXT    NOT NULL,
    payload         TEXT    NOT NULL,
    rationale       TEXT,
    confidence      REAL,
    status          TEXT    NOT NULL,
    applied_payload TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
INSERT INTO change_ops_copy
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops;

DROP TABLE change_ops;

CREATE TABLE change_ops (
    id              INTEGER PRIMARY KEY,
    changeset_id    INTEGER NOT NULL REFERENCES changesets (id),
    position        INTEGER NOT NULL,
    op_type         TEXT    NOT NULL CHECK (op_type IN (
                        'create_task', 'update_task', 'set_task_status', 'split_task',
                        'add_dependency', 'delete_task',
                        'create_blocker', 'update_blocker', 'resolve_blocker', 'delete_blocker',
                        'park_idea', 'update_idea', 'delete_idea',
                        'create_project', 'update_project', 'delete_project',
                        'create_milestone', 'update_milestone', 'delete_milestone')),
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    rationale       TEXT,
    confidence      REAL    CHECK (confidence BETWEEN 0 AND 1),
    status          TEXT    NOT NULL CHECK (status IN ('pending', 'applied', 'rejected', 'edited')),
    applied_payload TEXT    CHECK (json_valid(applied_payload)),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (changeset_id, position)
);

INSERT INTO change_ops
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops_copy;

DROP TABLE change_ops_copy;

-- +goose Down

-- Milestone ops have no place in the old table and are dropped, with any
-- pending links on them. Extraction doesn't propose milestone ops yet, so
-- they are only ever manual and applied, and no pending changeset loses an op.
-- The milestones themselves and tasks' milestone_id stay: both predate this
-- migration.
PRAGMA defer_foreign_keys = ON;

DELETE FROM pending_observation_links
WHERE change_op_id IN (SELECT id FROM change_ops
                       WHERE op_type IN ('create_milestone', 'update_milestone', 'delete_milestone'));

CREATE TABLE change_ops_copy (
    id              INTEGER PRIMARY KEY,
    changeset_id    INTEGER NOT NULL,
    position        INTEGER NOT NULL,
    op_type         TEXT    NOT NULL,
    payload         TEXT    NOT NULL,
    rationale       TEXT,
    confidence      REAL,
    status          TEXT    NOT NULL,
    applied_payload TEXT,
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
INSERT INTO change_ops_copy
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops
WHERE op_type NOT IN ('create_milestone', 'update_milestone', 'delete_milestone');

DROP TABLE change_ops;

CREATE TABLE change_ops (
    id              INTEGER PRIMARY KEY,
    changeset_id    INTEGER NOT NULL REFERENCES changesets (id),
    position        INTEGER NOT NULL,
    op_type         TEXT    NOT NULL CHECK (op_type IN (
                        'create_task', 'update_task', 'set_task_status', 'split_task',
                        'add_dependency', 'delete_task',
                        'create_blocker', 'update_blocker', 'resolve_blocker', 'delete_blocker',
                        'park_idea', 'update_idea', 'delete_idea',
                        'create_project', 'update_project', 'delete_project')),
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    rationale       TEXT,
    confidence      REAL    CHECK (confidence BETWEEN 0 AND 1),
    status          TEXT    NOT NULL CHECK (status IN ('pending', 'applied', 'rejected', 'edited')),
    applied_payload TEXT    CHECK (json_valid(applied_payload)),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (changeset_id, position)
);

INSERT INTO change_ops
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops_copy;

DROP TABLE change_ops_copy;
