-- Adds the settings table (PLAN 1.5.6): the user's preferences for this
-- database, one row with id 1. Each preference is its own typed column; later
-- ones arrive as new migrations adding columns with defaults. No row is
-- inserted here: at startup the server seeds one from config.toml (or the
-- built-in defaults) when it's missing, and never overwrites it afterwards.
-- The questions_per_entry bound must match domain.MaxQuestionsPerEntry.

-- +goose Up

CREATE TABLE settings (
    id                  INTEGER PRIMARY KEY CHECK (id = 1),
    active_project_cap  INTEGER NOT NULL CHECK (active_project_cap >= 1),
    questions_per_entry INTEGER NOT NULL CHECK (questions_per_entry BETWEEN 0 AND 2),
    created_at          INTEGER NOT NULL,
    updated_at          INTEGER NOT NULL
);

-- +goose Down

DROP TABLE settings;
