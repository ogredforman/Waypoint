-- Core schema from SPEC §6.1. Timestamps are INTEGER unix milliseconds (UTC),
-- set by Go. Enum CHECK lists must match the Go enums in internal/domain.

-- +goose Up

CREATE TABLE projects (
    id                 INTEGER PRIMARY KEY,
    title              TEXT    NOT NULL,
    why                TEXT    NOT NULL DEFAULT '',
    definition_of_done TEXT    NOT NULL DEFAULT '',
    status             TEXT    NOT NULL CHECK (status IN ('active', 'paused', 'parked', 'done')),
    created_at         INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
);
CREATE INDEX projects_status_idx ON projects (status);

-- Unused until post-MVP.
CREATE TABLE milestones (
    id          INTEGER PRIMARY KEY,
    project_id  INTEGER NOT NULL REFERENCES projects (id),
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL CHECK (status IN ('open', 'done', 'dropped')),
    position    INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);
CREATE INDEX milestones_project_id_idx ON milestones (project_id);

-- Immutable: see the entries_body_immutable trigger below.
CREATE TABLE entries (
    id                   INTEGER PRIMARY KEY,
    body                 TEXT    NOT NULL,
    project_id           INTEGER REFERENCES projects (id),
    reply_to_question_id INTEGER REFERENCES follow_up_questions (id),
    created_at           INTEGER NOT NULL
);
CREATE INDEX entries_project_id_created_at_idx ON entries (project_id, created_at);
CREATE INDEX entries_created_at_idx ON entries (created_at);
CREATE INDEX entries_reply_to_question_id_idx ON entries (reply_to_question_id);

-- +goose StatementBegin
CREATE TRIGGER entries_body_immutable
BEFORE UPDATE OF body ON entries
BEGIN
    SELECT RAISE(ABORT, 'entries.body is immutable');
END;
-- +goose StatementEnd

CREATE TABLE tasks (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER NOT NULL REFERENCES projects (id),
    milestone_id    INTEGER REFERENCES milestones (id),
    parent_task_id  INTEGER REFERENCES tasks (id),
    title           TEXT    NOT NULL,
    notes           TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL CHECK (status IN ('todo', 'in_progress', 'done', 'dropped')),
    size            TEXT    CHECK (size IN ('xs', 's', 'm', 'l')),
    energy          TEXT    CHECK (energy IN ('low', 'medium', 'deep')),
    position        INTEGER NOT NULL,
    origin_entry_id INTEGER REFERENCES entries (id),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    CHECK (parent_task_id IS NULL OR parent_task_id <> id)
);
CREATE INDEX tasks_project_id_idx ON tasks (project_id);
CREATE INDEX tasks_milestone_id_idx ON tasks (milestone_id);
CREATE INDEX tasks_parent_task_id_idx ON tasks (parent_task_id);
CREATE INDEX tasks_origin_entry_id_idx ON tasks (origin_entry_id);

CREATE TABLE task_dependencies (
    id                 INTEGER PRIMARY KEY,
    task_id            INTEGER NOT NULL REFERENCES tasks (id),
    blocked_by_task_id INTEGER NOT NULL REFERENCES tasks (id),
    created_at         INTEGER NOT NULL,
    UNIQUE (task_id, blocked_by_task_id),
    CHECK (task_id <> blocked_by_task_id)
);
CREATE INDEX task_dependencies_blocked_by_task_id_idx ON task_dependencies (blocked_by_task_id);

-- One extractor pass over an entry. Doubles as the job queue: pending runs
-- are picked up by the worker, and running ones reset to pending on restart.
CREATE TABLE extraction_runs (
    id             INTEGER PRIMARY KEY,
    entry_id       INTEGER NOT NULL REFERENCES entries (id),
    provider       TEXT    NOT NULL,
    model          TEXT    NOT NULL DEFAULT '',
    prompt_version TEXT    NOT NULL DEFAULT '',
    input_snapshot TEXT    CHECK (json_valid(input_snapshot)),
    raw_output     TEXT    CHECK (json_valid(raw_output)),
    status         TEXT    NOT NULL CHECK (status IN ('pending', 'running', 'succeeded', 'failed')),
    error          TEXT,
    latency_ms     INTEGER,
    created_at     INTEGER NOT NULL,
    updated_at     INTEGER NOT NULL
);
CREATE INDEX extraction_runs_entry_id_idx ON extraction_runs (entry_id);
CREATE INDEX extraction_runs_status_idx ON extraction_runs (status);

CREATE TABLE follow_up_questions (
    id                INTEGER PRIMARY KEY,
    entry_id          INTEGER NOT NULL REFERENCES entries (id),
    extraction_run_id INTEGER NOT NULL REFERENCES extraction_runs (id),
    text              TEXT    NOT NULL,
    reason            TEXT    NOT NULL DEFAULT '',
    status            TEXT    NOT NULL CHECK (status IN ('open', 'answered', 'dismissed')),
    answer_entry_id   INTEGER REFERENCES entries (id),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX follow_up_questions_entry_id_idx ON follow_up_questions (entry_id);
CREATE INDEX follow_up_questions_extraction_run_id_idx ON follow_up_questions (extraction_run_id);
CREATE INDEX follow_up_questions_status_idx ON follow_up_questions (status);
CREATE INDEX follow_up_questions_answer_entry_id_idx ON follow_up_questions (answer_entry_id);

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
CREATE INDEX observations_entry_id_idx ON observations (entry_id);
CREATE INDEX observations_extraction_run_id_idx ON observations (extraction_run_id);

-- target_id points into the table named by target_type, so it can't be a
-- foreign key. The changeset applier and validator keep it consistent.
CREATE TABLE observation_links (
    id             INTEGER PRIMARY KEY,
    observation_id INTEGER NOT NULL REFERENCES observations (id),
    target_type    TEXT    NOT NULL CHECK (target_type IN ('project', 'milestone', 'task', 'blocker', 'idea')),
    target_id      INTEGER NOT NULL,
    confidence     REAL    NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    created_at     INTEGER NOT NULL,
    UNIQUE (observation_id, target_type, target_id)
);
CREATE INDEX observation_links_target_idx ON observation_links (target_type, target_id);

CREATE TABLE blockers (
    id                INTEGER PRIMARY KEY,
    project_id        INTEGER NOT NULL REFERENCES projects (id),
    task_id           INTEGER REFERENCES tasks (id),
    description       TEXT    NOT NULL,
    status            TEXT    NOT NULL CHECK (status IN ('open', 'resolved')),
    origin_entry_id   INTEGER REFERENCES entries (id),
    resolved_entry_id INTEGER REFERENCES entries (id),
    created_at        INTEGER NOT NULL,
    updated_at        INTEGER NOT NULL
);
CREATE INDEX blockers_project_id_idx ON blockers (project_id);
CREATE INDEX blockers_task_id_idx ON blockers (task_id);
CREATE INDEX blockers_origin_entry_id_idx ON blockers (origin_entry_id);
CREATE INDEX blockers_resolved_entry_id_idx ON blockers (resolved_entry_id);

CREATE TABLE ideas (
    id              INTEGER PRIMARY KEY,
    project_id      INTEGER REFERENCES projects (id),
    title           TEXT    NOT NULL,
    body            TEXT    NOT NULL DEFAULT '',
    status          TEXT    NOT NULL CHECK (status IN ('parked', 'promoted', 'discarded')),
    origin_entry_id INTEGER REFERENCES entries (id),
    created_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL
);
CREATE INDEX ideas_project_id_idx ON ideas (project_id);
CREATE INDEX ideas_status_idx ON ideas (status);
CREATE INDEX ideas_origin_entry_id_idx ON ideas (origin_entry_id);

-- An extraction changeset has exactly one run; a manual one has none.
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
CREATE INDEX changesets_status_idx ON changesets (status);

CREATE TABLE change_ops (
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

-- One cached briefing per project, replaced on recompute.
CREATE TABLE briefings (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL UNIQUE REFERENCES projects (id),
    next_task_id INTEGER REFERENCES tasks (id),
    next_reason  TEXT    NOT NULL DEFAULT '',
    recap        TEXT    NOT NULL DEFAULT '',
    source       TEXT    NOT NULL CHECK (source IN ('heuristic', 'llm')),
    generated_at INTEGER NOT NULL,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);
CREATE INDEX briefings_next_task_id_idx ON briefings (next_task_id);

-- +goose Down

-- entries and follow_up_questions reference each other; defer the checks so
-- the drops don't trip over rows in the cycle. Applies to this transaction only.
PRAGMA defer_foreign_keys = ON;

DROP TABLE briefings;
DROP TABLE change_ops;
DROP TABLE changesets;
DROP TABLE ideas;
DROP TABLE blockers;
DROP TABLE observation_links;
DROP TABLE observations;
DROP TABLE follow_up_questions;
DROP TABLE extraction_runs;
DROP TABLE task_dependencies;
DROP TABLE tasks;
DROP TRIGGER entries_body_immutable;
DROP TABLE entries;
DROP TABLE milestones;
DROP TABLE projects;
