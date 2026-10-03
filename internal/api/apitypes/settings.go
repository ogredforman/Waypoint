package apitypes

import "github.com/ogredforman/waypoint/internal/domain"

// Settings is the body of GET and PATCH /api/settings: the user's preferences
// for this database. Technical settings (port, data dir, provider) stay in
// config.toml and aren't reported here.
type Settings struct {
	ActiveProjectCap int `json:"active_project_cap"`
	// QuestionsPerEntry is how many follow-up questions one entry may get, 0 to 2.
	QuestionsPerEntry int `json:"questions_per_entry"`
	// Theme is the colour theme.
	Theme domain.Theme `json:"theme"`
	// InterfaceFont is the font of the whole interface.
	InterfaceFont domain.InterfaceFont `json:"interface_font"`
	// DiaryFont is the font diary entries are written and read in.
	DiaryFont domain.DiaryFont `json:"diary_font"`
}

// UpdateSettingsRequest is the body of PATCH /api/settings. Only the fields
// present change; none may be null.
type UpdateSettingsRequest struct {
	ActiveProjectCap  *int                  `json:"active_project_cap,omitempty"`
	QuestionsPerEntry *int                  `json:"questions_per_entry,omitempty"`
	Theme             *domain.Theme         `json:"theme,omitempty"`
	InterfaceFont     *domain.InterfaceFont `json:"interface_font,omitempty"`
	DiaryFont         *domain.DiaryFont     `json:"diary_font,omitempty"`
}
