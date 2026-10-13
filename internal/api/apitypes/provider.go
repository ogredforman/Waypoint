package apitypes

// Provider is the body of GET and PUT /api/provider: the model provider
// settings every profile shares, saved in config.toml. The API key is never
// sent back, only whether one is set.
type Provider struct {
	// Provider is one of Providers; "mock" means no model.
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	Endpoint  string `json:"endpoint"`
	APIKeySet bool   `json:"api_key_set"`
	// FromEnv names the settings ("provider", "model", "endpoint",
	// "api_key") set by WAYPOINT_* env vars, which can't be changed here.
	FromEnv []string `json:"from_env"`
	// Providers are the provider names that can be chosen.
	Providers []string `json:"providers"`
}

// UpdateProviderRequest is the body of PUT /api/provider. Provider, model,
// and endpoint replace the current ones (empty for none). Without api_key the
// key is kept; an empty one removes it.
type UpdateProviderRequest struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Endpoint string  `json:"endpoint"`
	APIKey   *string `json:"api_key,omitempty"`
}
