-- Adds the font settings (PLAN 1.5.10): the interface font and the diary font
-- of this profile. The font files are bundled with the app; these columns only
-- name one. Existing rows get the defaults (Inter and Literata). The CHECK
-- lists must match domain.InterfaceFonts and domain.DiaryFonts. Later
-- appearance settings (the theme) arrive as their own migration the same way.

-- +goose Up

ALTER TABLE settings ADD COLUMN interface_font TEXT NOT NULL DEFAULT 'inter'
    CHECK (interface_font IN ('inter', 'geist', 'manrope', 'ibm-plex-sans', 'atkinson-hyperlegible-next'));

ALTER TABLE settings ADD COLUMN diary_font TEXT NOT NULL DEFAULT 'literata'
    CHECK (diary_font IN ('literata', 'inter', 'geist', 'manrope', 'ibm-plex-sans', 'atkinson-hyperlegible-next'));

-- +goose Down

ALTER TABLE settings DROP COLUMN diary_font;

ALTER TABLE settings DROP COLUMN interface_font;
