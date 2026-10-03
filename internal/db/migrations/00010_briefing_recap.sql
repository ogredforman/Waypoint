-- Makes a briefing's recap structured (PLAN 1.5.5e): instead of one string, the
-- recap is a few recent entries, each with short points of an observation kind,
-- so the Now screen can show them as a dated list. The entry's date is read
-- from the entry. Briefings are a cache, so the old ones are dropped; each is
-- regenerated the next time it is read. The kind CHECK list must match
-- domain.ObservationKinds.

-- +goose Up

CREATE TABLE briefing_recap_points (
    id          INTEGER PRIMARY KEY,
    briefing_id INTEGER NOT NULL REFERENCES briefings (id) ON DELETE CASCADE,
    entry_id    INTEGER NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    kind        TEXT    NOT NULL CHECK (kind IN ('progress', 'plan', 'blocker', 'idea', 'feeling', 'decision', 'question', 'note')),
    text        TEXT    NOT NULL,
    UNIQUE (briefing_id, position)
);
CREATE INDEX briefing_recap_points_entry_id_idx ON briefing_recap_points (entry_id);

DELETE FROM briefings;
ALTER TABLE briefings DROP COLUMN recap;

-- +goose Down

DROP TABLE briefing_recap_points;
DELETE FROM briefings;
ALTER TABLE briefings ADD COLUMN recap TEXT NOT NULL DEFAULT '';
