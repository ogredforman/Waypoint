-- Extends change_ops.op_type with the delete ops (ADR 0002) and the update ops
-- manual edits need (update_blocker, update_idea, update_project). SQLite can't
-- alter a CHECK constraint, so the table is rebuilt. Nothing references
-- change_ops, so dropping and renaming it is safe with foreign keys on.
-- The CHECK list must match domain.OpTypes(), values and order.

-- +goose Up

CREATE TABLE change_ops_new (
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

INSERT INTO change_ops_new
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops;

DROP TABLE change_ops;
ALTER TABLE change_ops_new RENAME TO change_ops;

-- +goose Down

-- Ops of the types added above have no place in the old table and are dropped.
CREATE TABLE change_ops_old (
    id              INTEGER PRIMARY KEY,
    changeset_id    INTEGER NOT NULL REFERENCES changesets (id),
    position        INTEGER NOT NULL,
    op_type         TEXT    NOT NULL CHECK (op_type IN (
                        'create_task', 'update_task', 'set_task_status', 'split_task',
                        'create_blocker', 'resolve_blocker', 'park_idea', 'create_project',
                        'add_dependency')),
    payload         TEXT    NOT NULL CHECK (json_valid(payload)),
    rationale       TEXT,
    confidence      REAL    CHECK (confidence BETWEEN 0 AND 1),
    status          TEXT    NOT NULL CHECK (status IN ('pending', 'applied', 'rejected', 'edited')),
    applied_payload TEXT    CHECK (json_valid(applied_payload)),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    UNIQUE (changeset_id, position)
);

INSERT INTO change_ops_old
SELECT id, changeset_id, position, op_type, payload, rationale, confidence, status,
       applied_payload, created_at, updated_at
FROM change_ops
WHERE op_type IN (
    'create_task', 'update_task', 'set_task_status', 'split_task',
    'create_blocker', 'resolve_blocker', 'park_idea', 'create_project',
    'add_dependency');

DROP TABLE change_ops;
ALTER TABLE change_ops_old RENAME TO change_ops;
