package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

// TestFixtures loads testdata/entries, which checks every expected result
// against its entry and snapshot.
func TestFixtures(t *testing.T) {
	fs, err := LoadFixtures("../../testdata/entries")
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) < 15 || len(fs) > 30 {
		t.Errorf("got %d fixtures, want 15 to 30", len(fs))
	}

	kinds := map[domain.ObservationKind]bool{}
	ops := map[domain.OpType]bool{}
	var replies, unscoped, empty int
	for _, f := range fs {
		if f.Input.Entry.Body == "" {
			t.Errorf("%s: empty entry", f.Name)
		}
		if f.Notes == "" {
			t.Errorf("%s: no notes", f.Name)
		}
		for _, o := range f.Expected.Observations {
			kinds[o.Kind] = true
		}
		for _, op := range f.Expected.Ops {
			ops[op.OpType] = true
		}
		if f.Input.ReplyTo != nil {
			replies++
		}
		if f.Input.Entry.ProjectID == nil {
			unscoped++
		}
		if len(f.Expected.Ops) == 0 {
			empty++
		}
	}
	for _, k := range domain.ObservationKinds() {
		if !kinds[k] {
			t.Errorf("no fixture expects a %s observation", k)
		}
	}
	for _, op := range []domain.OpType{
		domain.OpCreateTask, domain.OpSetTaskStatus, domain.OpSplitTask, domain.OpAddDependency,
		domain.OpCreateBlocker, domain.OpResolveBlocker, domain.OpParkIdea, domain.OpUpdateIdea,
		domain.OpCreateProject, domain.OpCreateMilestone, domain.OpUpdateMilestone,
	} {
		if !ops[op] {
			t.Errorf("no fixture expects a %s op", op)
		}
	}
	if replies == 0 || unscoped == 0 || empty == 0 {
		t.Errorf("want replies, unscoped entries, and entries with no ops; got %d, %d, %d", replies, unscoped, empty)
	}
}

func TestLoadFixturesRefusesBadQuote(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "snapshots/empty.json", `{"taken_at": "2026-10-12T21:00:00Z"}`)
	write(t, dir, "01-x.md", "Shipped it.\n")
	write(t, dir, "01-x.json", `{"snapshot": "empty", "notes": "n", "expected": {
		"observations": [{"kind": "progress", "summary": "s", "quote": "not in the entry"}], "ops": [], "questions": []}}`)
	if _, err := LoadFixtures(dir); err == nil {
		t.Fatal("want an error for a quote that isn't in the entry")
	}
}

func TestLoadFixturesRefusesForbiddenExpectedOp(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "snapshots/empty.json", `{"taken_at": "2026-10-12T21:00:00Z"}`)
	write(t, dir, "01-x.md", "New project: a shed.\n")
	write(t, dir, "01-x.json", `{"snapshot": "empty", "notes": "n", "expected": {"observations": [], "questions": [],
		"ops": [{"op_type": "create_project", "payload": {"title": "Shed"}}]}, "forbid": ["create_project"]}`)
	if _, err := LoadFixtures(dir); err == nil {
		t.Fatal("want an error for an expected op the fixture forbids")
	}
}

func TestLoadFixturesRefusesUnknownID(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "snapshots/empty.json", `{"taken_at": "2026-10-12T21:00:00Z"}`)
	write(t, dir, "01-x.md", "Done with it.\n")
	write(t, dir, "01-x.json", `{"snapshot": "empty", "notes": "n", "expected": {"observations": [], "questions": [],
		"ops": [{"op_type": "set_task_status", "payload": {"task_id": 7, "status": "done"}}]}}`)
	if _, err := LoadFixtures(dir); err == nil {
		t.Fatal("want an error for an expected op naming a task the snapshot doesn't have")
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
