package api

import (
	"context"
	"mime"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Provider holds the model provider settings every profile shares;
// provider.Switch implements it. Update returns a *domain.ValidationError
// for settings it refuses, and changes nothing then.
type Provider interface {
	Settings() domain.ProviderSettings
	Update(ctx context.Context, u domain.ProviderUpdate) (domain.ProviderSettings, error)
}

func providerDTO(s domain.ProviderSettings) apitypes.Provider {
	return apitypes.Provider{
		Provider:  s.Provider,
		Model:     s.Model,
		Endpoint:  s.Endpoint,
		APIKeySet: s.APIKeySet,
		FromEnv:   append([]string{}, s.FromEnv...),
		Providers: append([]string{}, s.Providers...),
	}
}

// providerRoutes mounts /provider on the /api router.
func (h *handlers) providerRoutes(r chi.Router, p Provider) {
	r.Get("/provider", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, providerDTO(p.Settings()))
	})
	r.Put("/provider", func(w http.ResponseWriter, r *http.Request) {
		// A web page can send a text/plain PUT to 127.0.0.1 without a CORS
		// preflight; a JSON one needs a preflight, which the server never
		// grants. So no other site can point the endpoint and key elsewhere.
		if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "the body must be application/json")
			return
		}
		var req apitypes.UpdateProviderRequest
		if err := readJSON(w, r, &req); err != nil {
			h.fail(w, r, err)
			return
		}
		s, err := p.Update(r.Context(), domain.ProviderUpdate{
			Provider: req.Provider,
			Model:    req.Model,
			Endpoint: req.Endpoint,
			APIKey:   req.APIKey,
		})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, providerDTO(s))
	})
}
