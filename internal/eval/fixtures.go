// Package eval runs the extraction fixtures in testdata/entries against an
// extractor and scores what it proposes.
package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// A Fixture is one sample entry: the input an extractor gets and the result
// we expect back. Expected is a complete result that passes extract.Validate;
// its free text (summaries, titles, rationales) is one good answer, not the
// only one.
type Fixture struct {
	Name     string // file name without extension, e.g. "03-park-git-idea"
	Notes    string // what the fixture tests
	Input    extract.ExtractInput
	Expected extract.ExtractionResult
	Forbid   []domain.OpType // op types a good result never contains
}

// fixtureFile is NN-slug.json; the entry body is NN-slug.md beside it.
type fixtureFile struct {
	Snapshot  string                   `json:"snapshot"` // snapshots/<name>.json
	ProjectID *int64                   `json:"project_id"`
	ReplyTo   *replyTo                 `json:"reply_to"`
	Asked     []asked                  `json:"asked"`
	Notes     string                   `json:"notes"`
	Expected  extract.ExtractionResult `json:"expected"`
	Forbid    []domain.OpType          `json:"forbid"`
}

type replyTo struct {
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// asked is a follow-up question asked after one of the snapshot's recent
// entries, added to that entry for this fixture only.
type asked struct {
	Entry  int64                 `json:"entry"`
	Text   string                `json:"text"`
	Status domain.QuestionStatus `json:"status"`
}

// Entry and question IDs the fixtures are given; nothing in a snapshot uses them.
const (
	firstEntryID = 1000
	questionID   = 900
)

// LoadFixtures reads every fixture in dir, in file name order. It refuses a
// fixture whose expected result doesn't hold up: a quote missing from the
// body, a result extract.Validate rejects, or a forbidden op type the
// expected ops use.
func LoadFixtures(dir string) ([]Fixture, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	snapshots := map[string]extract.StateSnapshot{}
	var out []Fixture
	for i, path := range names {
		f, err := loadFixture(dir, path, int64(firstEntryID+i), snapshots)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		out = append(out, f)
	}
	return out, nil
}

func loadFixture(dir, path string, entryID int64, snapshots map[string]extract.StateSnapshot) (Fixture, error) {
	var ff fixtureFile
	if err := decodeFile(path, &ff); err != nil {
		return Fixture{}, err
	}
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	body, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		return Fixture{}, err
	}
	snap, ok := snapshots[ff.Snapshot]
	if !ok {
		if err := decodeFile(filepath.Join(dir, "snapshots", ff.Snapshot+".json"), &snap); err != nil {
			return Fixture{}, fmt.Errorf("snapshot %q: %w", ff.Snapshot, err)
		}
		snapshots[ff.Snapshot] = snap
	}

	snap, err = withAsked(snap, ff.Asked)
	if err != nil {
		return Fixture{}, err
	}

	in := extract.ExtractInput{
		Entry: domain.Entry{
			ID:        entryID,
			Body:      strings.TrimSuffix(string(body), "\n"),
			ProjectID: ff.ProjectID,
			CreatedAt: snap.TakenAt,
		},
		Snapshot:      forEntry(snap, ff.ProjectID),
		MaxQuestions:  extract.MaxQuestions,
		PromptVersion: extract.PromptVersion,
	}
	if ff.ReplyTo != nil {
		in.Entry.ReplyToQuestionID = ptr(int64(questionID))
		in.ReplyTo = &domain.FollowUpQuestion{
			ID: questionID, EntryID: entryID - 1, Text: ff.ReplyTo.Text, Reason: ff.ReplyTo.Reason,
			Status: domain.QuestionOpen, CreatedAt: snap.TakenAt.Add(-24 * time.Hour), UpdatedAt: snap.TakenAt.Add(-24 * time.Hour),
		}
	}

	exp := ff.Expected
	extract.ResolveQuotes(in.Entry.Body, exp.Observations)
	for i, o := range exp.Observations {
		if o.Quote != "" && o.SpanEnd == 0 {
			return Fixture{}, fmt.Errorf("observations[%d]: quote %q isn't in the entry", i, o.Quote)
		}
	}
	if err := extract.Validate(in, exp); err != nil {
		return Fixture{}, fmt.Errorf("expected result: %w", err)
	}
	for _, t := range ff.Forbid {
		if !slices.Contains(domain.OpTypes(), t) {
			return Fixture{}, fmt.Errorf("forbid: unknown op type %q", t)
		}
		for _, op := range exp.Ops {
			if op.OpType == t {
				return Fixture{}, fmt.Errorf("forbid: expected ops use %q", t)
			}
		}
	}
	return Fixture{Name: name, Notes: ff.Notes, Input: in, Expected: exp, Forbid: ff.Forbid}, nil
}

// withAsked returns snap with the questions in asked added to its recent
// entries. The shared snapshot is left alone: the recent entries are copied.
func withAsked(snap extract.StateSnapshot, asked []asked) (extract.StateSnapshot, error) {
	if len(asked) == 0 {
		return snap, nil
	}
	snap.RecentEntries = slices.Clone(snap.RecentEntries)
	for i, a := range asked {
		if !a.Status.Valid() {
			return snap, fmt.Errorf("asked[%d]: unknown status %q", i, a.Status)
		}
		j := slices.IndexFunc(snap.RecentEntries, func(e extract.SnapshotEntry) bool { return e.ID == a.Entry })
		if j < 0 {
			return snap, fmt.Errorf("asked[%d]: entry %d isn't a recent entry of the snapshot", i, a.Entry)
		}
		e := &snap.RecentEntries[j]
		e.Questions = append(slices.Clone(e.Questions), extract.SnapshotQuestion{Text: a.Text, Status: a.Status})
	}
	return snap, nil
}

// forEntry trims a snapshot the way the pipeline builds one: active projects,
// plus the entry's own project whatever its status.
func forEntry(snap extract.StateSnapshot, projectID *int64) extract.StateSnapshot {
	projects := snap.Projects
	snap.Projects = nil
	for _, p := range projects {
		if p.Status == domain.ProjectActive || (projectID != nil && p.ID == *projectID) {
			snap.Projects = append(snap.Projects, p)
		}
	}
	return snap
}

func decodeFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func ptr[T any](v T) *T { return &v }
