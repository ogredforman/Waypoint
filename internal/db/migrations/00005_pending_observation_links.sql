-- Adds pending observation links (PLAN 1.5.5): a link from an observation to
-- an entity that an op of the same changeset creates ("$new:N"). It names the
-- creating op instead of a target row. When that op is applied, the applier
-- turns it into an observation_links row for the new entity and deletes it;
-- when the op is rejected, it is deleted. The target_type CHECK list must
-- match domain.TargetTypes(), values and order.

-- +goose Up

CREATE TABLE pending_observation_links (
    id             INTEGER PRIMARY KEY,
    observation_id INTEGER NOT NULL REFERENCES observations (id),
    change_op_id   INTEGER NOT NULL REFERENCES change_ops (id),
    target_type    TEXT    NOT NULL CHECK (target_type IN ('project', 'milestone', 'task', 'blocker', 'idea')),
    confidence     REAL    NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    created_at     INTEGER NOT NULL,
    UNIQUE (observation_id, change_op_id)
);
CREATE INDEX pending_observation_links_op_idx ON pending_observation_links (change_op_id);

-- +goose Down

DROP TABLE pending_observation_links;
