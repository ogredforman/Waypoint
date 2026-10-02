package domain

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
}

// DefaultSettings are the settings of a database that has none stored and no
// config values to seed them.
func DefaultSettings() Settings {
	return Settings{ActiveProjectCap: DefaultActiveProjectCap, QuestionsPerEntry: MaxQuestionsPerEntry}
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
	return c.err("settings")
}
