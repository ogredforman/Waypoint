package domain

// ProviderSettings are the model provider settings in effect, shared by
// every profile. The API key itself is never reported, only whether one is
// set.
type ProviderSettings struct {
	Provider  string // empty means the mock
	Model     string
	Endpoint  string
	APIKeySet bool
	// FromEnv names the settings ("provider", "model", "endpoint",
	// "api_key") set by WAYPOINT_* env vars. Those win over config.toml, so
	// they can't be changed from the app.
	FromEnv []string
	// Providers are the provider names that can be chosen.
	Providers []string
}

// ProviderUpdate is a change to the provider settings: Provider, Model, and
// Endpoint replace the current ones (empty for none); a nil APIKey keeps the
// current key and an empty one removes it.
type ProviderUpdate struct {
	Provider string
	Model    string
	Endpoint string
	APIKey   *string
}
