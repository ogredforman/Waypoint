package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/api/apitypes"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

func TestExport(t *testing.T) {
	s := newServer(t)
	p := s.project("Build Waypoint", domain.ProjectActive)
	if rec := s.call("POST", "/api/profiles/t/entries", map[string]any{"body": "Wrote the export.", "project_id": p.ID}); rec.Code != http.StatusAccepted {
		t.Fatalf("create entry: %d %s", rec.Code, rec.Body)
	}

	rec := s.call("GET", "/api/profiles/t/export", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("export: %d %s", rec.Code, rec.Body)
	}
	// The migration names the profile "Personal".
	if got := rec.Header().Get("Content-Disposition"); !regexp.MustCompile(`^attachment; filename="waypoint-personal-\d{4}-\d{2}-\d{2}\.json"$`).MatchString(got) {
		t.Errorf("Content-Disposition %q", got)
	}
	var out apitypes.Export
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Format != "waypoint-export" || out.Version != 1 || out.SchemaVersion < 13 || out.ExportedAt.IsZero() {
		t.Errorf("envelope %+v", out)
	}
	if !slices.Equal(out.TableOrder, store.ExportTables()) {
		t.Errorf("table_order %v", out.TableOrder)
	}
	for _, table := range out.TableOrder {
		if out.Tables[table] == nil {
			t.Errorf("table %s missing or null", table)
		}
	}
	projects := out.Tables["projects"]
	if len(projects) != 1 || projects[0]["title"] != "Build Waypoint" || projects[0]["id"] != float64(p.ID) {
		t.Errorf("projects %v", projects)
	}
	entries := out.Tables["entries"]
	if len(entries) != 1 || entries[0]["body"] != "Wrote the export." {
		t.Errorf("entries %v", entries)
	}
	if len(out.Tables["profile"]) != 1 || len(out.Tables["settings"]) != 1 {
		t.Errorf("profile %v, settings %v", out.Tables["profile"], out.Tables["settings"])
	}
	// The entry's run is stored with it, whatever its state.
	if len(out.Tables["extraction_runs"]) != 1 {
		t.Errorf("extraction_runs %v", out.Tables["extraction_runs"])
	}
}
