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
