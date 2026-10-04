-- Adds the delete_project op type (PLAN 1.5.13, ADR 0008). SQLite can't alter
-- a CHECK constraint, so change_ops is rebuilt.
--
-- pending_observation_links references change_ops, so the old rows are copied
-- aside, the table dropped and recreated under its own name, and the rows
-- copied back. Foreign key checks are deferred to commit (this transaction
-- only); by then every pending link has its op again. The columns keep their
-- order, since the generated queries select *. The CHECK list must match
-- domain.OpTypes(), values and order.

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

-- +goose Down

-- delete_project ops have no place in the old table and are dropped, with
-- any pending links on them. A delete_project op is only ever manual and
-- applied, so no pending changeset loses an op.
PRAGMA defer_foreign_keys = ON;

DELETE FROM pending_observation_links
WHERE change_op_id IN (SELECT id FROM change_ops WHERE op_type = 'delete_project');

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
WHERE op_type <> 'delete_project';

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
                        'create_project', 'update_project')),
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
