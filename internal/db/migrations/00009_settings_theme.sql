-- Adds the theme setting (PLAN 1.5.11): the colour theme of this profile, one
-- of the themes the frontend defines in app.css. Existing rows get Midnight,
-- the dark purple theme the app had before. The CHECK list must match
-- domain.Themes.

-- +goose Up

ALTER TABLE settings ADD COLUMN theme TEXT NOT NULL DEFAULT 'midnight'
    CHECK (theme IN ('midnight', 'forest', 'paper', 'sage', 'dawn'));

-- +goose Down

ALTER TABLE settings DROP COLUMN theme;
