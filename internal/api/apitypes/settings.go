package apitypes

// Settings is the body of GET and PATCH /api/settings: the user's preferences
// for this database. Technical settings (port, data dir, provider) stay in
// config.toml and aren't reported here.
type Settings struct {
	ActiveProjectCap int `json:"active_project_cap"`
	// QuestionsPerEntry is how many follow-up questions one entry may get, 0 to 2.
	QuestionsPerEntry int `json:"questions_per_entry"`
}

// UpdateSettingsRequest is the body of PATCH /api/settings. Only the fields
// present change; neither may be null.
type UpdateSettingsRequest struct {
	ActiveProjectCap  *int `json:"active_project_cap,omitempty"`
	QuestionsPerEntry *int `json:"questions_per_entry,omitempty"`
}
