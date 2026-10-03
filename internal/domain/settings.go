package domain

import "strings"

// MaxQuestionsPerEntry is the most follow-up questions one entry may ever get
// (SPEC §5). The questions-per-entry setting can lower it, not raise it.
const MaxQuestionsPerEntry = 2

// Settings are the user's preferences for one database (SPEC §10.5). Technical
// settings such as the port stay in config.toml.
type Settings struct {
	// ActiveProjectCap limits how many projects can be active at once.
	ActiveProjectCap int
	// QuestionsPerEntry is how many follow-up questions one entry may get,
	// from 0 to MaxQuestionsPerEntry.
	QuestionsPerEntry int

	// Appearance. CopyAppearance lists these fields too.

	// Theme is the colour theme.
	Theme Theme
	// InterfaceFont is the font of the whole interface.
	InterfaceFont InterfaceFont
	// DiaryFont is the font diary entries are written and read in.
	DiaryFont DiaryFont
}

// DefaultSettings are the settings of a database that has none stored and no
// config values to seed them.
func DefaultSettings() Settings {
	return Settings{
		ActiveProjectCap:  DefaultActiveProjectCap,
		QuestionsPerEntry: MaxQuestionsPerEntry,
		Theme:             ThemeMidnight,
		InterfaceFont:     FontInter,
		DiaryFont:         DiaryLiterata,
	}
}

// Validate reports values out of range. Field names are the API's.
func (s Settings) Validate() error {
	var c checker
	if s.ActiveProjectCap < 1 {
		c.add("active_project_cap", "must be at least 1, got %d", s.ActiveProjectCap)
	}
	if s.QuestionsPerEntry < 0 || s.QuestionsPerEntry > MaxQuestionsPerEntry {
		c.add("questions_per_entry", "must be from 0 to %d, got %d", MaxQuestionsPerEntry, s.QuestionsPerEntry)
	}
	if !s.Theme.Valid() {
		c.add("theme", "must be one of %s", list(Themes()))
	}
	if !s.InterfaceFont.Valid() {
		c.add("interface_font", "must be one of %s", list(InterfaceFonts()))
	}
	if !s.DiaryFont.Valid() {
		c.add("diary_font", "must be one of %s", list(DiaryFonts()))
	}
	return c.err("settings")
}

// CopyAppearance returns s with from's appearance (theme and fonts), the
// other settings unchanged. A new profile starts with the appearance of the
// one it was created from.
func (s Settings) CopyAppearance(from Settings) Settings {
	s.Theme = from.Theme
	s.InterfaceFont = from.InterfaceFont
	s.DiaryFont = from.DiaryFont
	return s
}

// list joins enum values for a problem message, without the rejected value.
func list[T ~string](values []T) string {
	s := make([]string, len(values))
	for i, v := range values {
		s[i] = string(v)
	}
	return strings.Join(s, ", ")
}
