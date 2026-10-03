package api

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Profiles is the set of profiles the server runs (ADR 0005). Each profile is
// its own database with its own store, worker, hub, and briefings;
// profile.Manager implements this. Errors: store.ErrNotFound for an unknown
// profile, *domain.ValidationError for a bad or duplicate name, and
// domain.ErrLastProfile when deleting the only profile.
type Profiles interface {
	// List returns the profiles in creation order.
	List() []domain.Profile
	// Create makes a profile named name. A non-empty appearanceFrom is the
	// ID of the profile whose appearance (theme and fonts) it starts with.
	Create(ctx context.Context, name, appearanceFrom string) (domain.Profile, error)
	Rename(ctx context.Context, id, name string) (domain.Profile, error)
	// Delete stops the profile and removes its database files.
	Delete(ctx context.Context, id string) error
	// Serve handles r with profile id's handler (from NewProfileHandler) and
	// reports whether the profile exists. The route path is set to the part
	// after /api/profiles/{profile}.
	Serve(id string, w http.ResponseWriter, r *http.Request) bool
	// Ping reports whether every profile's database can be reached.
	Ping(ctx context.Context) error
}

func profileDTO(p domain.Profile) apitypes.Profile {
	return apitypes.Profile{ID: p.ID, Name: p.Name}
}

// profileRoutes mounts the profile list and every profile's own routes on
// the /api router.
func (h *handlers) profileRoutes(r chi.Router, ps Profiles) {
	r.Get("/profiles", func(w http.ResponseWriter, r *http.Request) {
		list := ps.List()
		out := make([]apitypes.Profile, len(list))
		for i, p := range list {
			out[i] = profileDTO(p)
		}
		writeJSON(w, http.StatusOK, out)
	})
	r.Post("/profiles", func(w http.ResponseWriter, r *http.Request) {
		var req apitypes.CreateProfileRequest
		if err := readJSON(w, r, &req); err != nil {
			h.fail(w, r, err)
			return
		}
		p, err := ps.Create(r.Context(), req.Name, req.AppearanceFrom)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, profileDTO(p))
	})
	r.Patch("/profiles/{profile}", func(w http.ResponseWriter, r *http.Request) {
		var req apitypes.RenameProfileRequest
		if err := readJSON(w, r, &req); err != nil {
			h.fail(w, r, err)
			return
		}
		p, err := ps.Rename(r.Context(), chi.URLParam(r, "profile"), req.Name)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, profileDTO(p))
	})
	r.Delete("/profiles/{profile}", func(w http.ResponseWriter, r *http.Request) {
		if err := ps.Delete(r.Context(), chi.URLParam(r, "profile")); err != nil {
			h.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	// Everything else under a profile is that profile's own router.
	r.Handle("/profiles/{profile}/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		rctx.RoutePath = "/" + chi.URLParam(r, "*")
		if !ps.Serve(chi.URLParam(r, "profile"), w, r) {
			writeError(w, http.StatusNotFound, "not_found", "no such profile")
		}
	}))
}
