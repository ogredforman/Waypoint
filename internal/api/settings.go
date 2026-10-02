package api

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

func settingsDTO(s domain.Settings) apitypes.Settings {
	return apitypes.Settings{ActiveProjectCap: s.ActiveProjectCap, QuestionsPerEntry: s.QuestionsPerEntry}
}

func (h *handlers) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.store.GetSettings(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsDTO(s))
}

// patchSettings changes the settings present in the body (an
// apitypes.UpdateSettingsRequest) and answers with all of them. Settings
// aren't projects, tasks, blockers, or ideas, so they're written directly,
// not as a changeset. A lower active-project cap deactivates nothing; it
// applies to the next activation.
func (h *handlers) patchSettings(w http.ResponseWriter, r *http.Request) {
	body, err := readObject(w, r)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if len(body) == 0 {
		h.fail(w, r, invalid("settings", "request", "set at least one setting"))
		return
	}
	var updated domain.Settings
	err = h.store.InTx(r.Context(), func(tx *store.Tx) error {
		s, err := tx.GetSettings(r.Context())
		if err != nil {
			return err
		}
		var problems []domain.FieldError
		for _, name := range slices.Sorted(maps.Keys(body)) {
			raw := body[name]
			var dst *int
			switch name {
			case "active_project_cap":
				dst = &s.ActiveProjectCap
			case "questions_per_entry":
				dst = &s.QuestionsPerEntry
			default:
				problems = append(problems, domain.FieldError{Field: name, Problem: "is not a known field"})
				continue
			}
			var v *int
			if json.Unmarshal(raw, &v) != nil || v == nil {
				problems = append(problems, domain.FieldError{Field: name, Problem: "must be a whole number"})
				continue
			}
			*dst = *v
		}
		if len(problems) > 0 {
			return &domain.ValidationError{Entity: "settings", Problems: problems}
		}
		updated, err = tx.UpdateSettings(r.Context(), s)
		return err
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, settingsDTO(updated))
}
