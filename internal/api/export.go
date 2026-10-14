package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// exportFormat and exportVersion identify the envelope of GET /api/export.
const (
	exportFormat  = "waypoint-export"
	exportVersion = 1
)

// getExport answers with the whole profile database as an attachment, read
// in one transaction so the tables agree with each other.
func (h *handlers) getExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := apitypes.Export{
		Format:     exportFormat,
		Version:    exportVersion,
		ExportedAt: time.Now().UTC(),
		Tables:     map[string][]map[string]any{},
		TableOrder: store.ExportTables(),
	}
	var name string
	err := h.store.InReadTx(ctx, func(tx *store.ReadTx) error {
		v, err := tx.SchemaVersion(ctx)
		if err != nil {
			return err
		}
		out.SchemaVersion = v
		p, err := tx.GetProfile(ctx)
		if err != nil {
			return err
		}
		name = p.Name
		for _, table := range out.TableOrder {
			rows, err := tx.ExportTable(ctx, table)
			if err != nil {
				return err
			}
			out.Tables[table] = rows
		}
		return nil
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	filename := fmt.Sprintf("waypoint-%s-%s.json", domain.ProfileSlug(name), out.ExportedAt.Format("2006-01-02"))
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	writeJSON(w, http.StatusOK, out)
}
