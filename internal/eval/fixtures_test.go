package eval

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
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
	var replies, unscoped, empty, questions, asked int
	for _, f := range fs {
		if len(f.Expected.Questions) > 0 {
			questions++
		}
		for _, e := range f.Input.Snapshot.RecentEntries {
			if len(e.Questions) > 0 {
				asked++
			}
		}
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
	// One fixture per question gap (SPEC §7.2), and one where a question was
	// recently asked; only that fixture sees it.
	if questions < 3 || asked != 1 {
		t.Errorf("want 3+ fixtures expecting a question and 1 with a recently asked one; got %d, %d", questions, asked)
	}
}

func TestLoadFixturesAsked(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "snapshots/s.json", `{"taken_at": "2026-10-12T21:00:00Z",
		"recent_entries": [{"id": 5, "created_at": "2026-10-11T21:00:00Z", "summaries": ["Did a thing"]}]}`)
	body := "Did another thing.\n"
	empty := `"expected": {"observations": [], "ops": [], "questions": []}`
	write(t, dir, "01-a.md", body)
	write(t, dir, "01-a.json", `{"snapshot": "s", "notes": "n", `+empty+`,
		"asked": [{"entry": 5, "text": "What's next?", "status": "dismissed"}]}`)
	write(t, dir, "02-b.md", body)
	write(t, dir, "02-b.json", `{"snapshot": "s", "notes": "n", `+empty+`}`)
	fs, err := LoadFixtures(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []extract.SnapshotQuestion{{Text: "What's next?", Status: domain.QuestionDismissed}}
	if got := fs[0].Input.Snapshot.RecentEntries[0].Questions; !reflect.DeepEqual(got, want) {
		t.Errorf("01-a questions = %+v, want %+v", got, want)
	}
	if got := fs[1].Input.Snapshot.RecentEntries[0].Questions; got != nil {
		t.Errorf("02-b questions = %+v, want none: asked is per fixture", got)
	}

	write(t, dir, "03-c.md", body)
	write(t, dir, "03-c.json", `{"snapshot": "s", "notes": "n", `+empty+`,
		"asked": [{"entry": 6, "text": "What's next?", "status": "open"}]}`)
	if _, err := LoadFixtures(dir); err == nil {
		t.Fatal("want an error for a question asked after an entry the snapshot doesn't have")
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
