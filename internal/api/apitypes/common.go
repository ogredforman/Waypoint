package apitypes

// ErrorBody is the body of every failed API request.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail says what went wrong. Code is stable and meant for code;
// Message is for people. Problems lists the fields at fault when Code is
// "invalid".
type ErrorDetail struct {
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Problems []FieldProblem `json:"problems,omitempty"`
}

// FieldProblem is one problem with one field of the request.
type FieldProblem struct {
	Field   string `json:"field"`
	Problem string `json:"problem"`
}

// Health is the body of GET /api/health, with status 200 or 503.
type Health struct {
	Status string `json:"status"` // "ok" or "unavailable"
}

// Settings is the body of GET /api/settings: the settings in effect, read
// from config.toml. They can't be changed from the app yet.
type Settings struct {
	// Provider names the extractor turning entries into changesets.
	Provider              string `json:"provider"`
	ActiveProjectCap      int    `json:"active_project_cap"`
	SnapshotRecentEntries int    `json:"snapshot_recent_entries"`
	// MockLatencyMS is the mock extractor's artificial delay per run.
	MockLatencyMS int64 `json:"mock_latency_ms"`
}
