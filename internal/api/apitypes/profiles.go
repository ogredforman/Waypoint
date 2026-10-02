package apitypes

// Profile is one of the user's profiles (ADR 0005): a separate database with
// a display name. ID names it in API paths (/api/profiles/{id}/...) and never
// changes; Name can be changed.
type Profile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateProfileRequest is the body of POST /api/profiles.
type CreateProfileRequest struct {
	Name string `json:"name"`
}

// RenameProfileRequest is the body of PATCH /api/profiles/{profile}.
type RenameProfileRequest struct {
	Name string `json:"name"`
}
