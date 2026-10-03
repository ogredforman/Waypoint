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
	return apitypes.Settings{
		ActiveProjectCap:  s.ActiveProjectCap,
		QuestionsPerEntry: s.QuestionsPerEntry,
		Theme:             s.Theme,
		InterfaceFont:     s.InterfaceFont,
		DiaryFont:         s.DiaryFont,
	}
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
			var problem string
			switch name {
			case "active_project_cap":
				problem = decodeField(raw, &s.ActiveProjectCap, "must be a whole number")
			case "questions_per_entry":
				problem = decodeField(raw, &s.QuestionsPerEntry, "must be a whole number")
			case "theme":
				problem = decodeField(raw, &s.Theme, "must be a theme name")
			case "interface_font":
				problem = decodeField(raw, &s.InterfaceFont, "must be a font name")
			case "diary_font":
				problem = decodeField(raw, &s.DiaryFont, "must be a font name")
			default:
				problem = "is not a known field"
			}
			if problem != "" {
				problems = append(problems, domain.FieldError{Field: name, Problem: problem})
			}
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

// decodeField sets *dst from raw, a settings field that may not be null. It
// answers with the problem (wrongType when raw isn't a T), or "" when dst was
// set. Ranges and allowed values are left to domain.Settings.Validate.
func decodeField[T any](raw json.RawMessage, dst *T, wrongType string) string {
	var v *T
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return wrongType
	}
	*dst = *v
	return ""
}
