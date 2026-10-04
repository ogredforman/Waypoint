package changeset

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/store"
)

// scopedEntries adds two entries scoped to projectID, each with a run in
// status runStatus, and returns their IDs.
func scopedEntries(t *testing.T, f fixture, projectID int64, runStatus string) []int64 {
	t.Helper()
	ctx := context.Background()
	var ids []int64
	for _, body := range []string{"first", "second"} {
		var e domain.Entry
		err := f.s.InTx(ctx, func(tx *store.Tx) (err error) {
			e, err = tx.CreateEntry(ctx, domain.Entry{Body: body, ProjectID: &projectID})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.ExecContext(ctx, `INSERT INTO extraction_runs (entry_id, provider, status, created_at, updated_at)
			VALUES (?, 'mock', ?, 1, 1)`, e.ID, runStatus); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	return ids
}

func TestApplyManualDeleteProject(t *testing.T) {
	ctx := context.Background()
	for _, tt := range []struct {
		name          string
		deleteEntries bool
	}{
		{"keep entries", false},
		{"delete entries", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			entries := scopedEntries(t, f, f.projA, "succeeded")
			var after recorder
			res, err := NewApplier(f.s, &after).ApplyManual(ctx, DeleteProject{ProjectID: ID(f.projA), DeleteEntries: tt.deleteEntries})
			if err != nil {
				t.Fatal(err)
			}

			if _, err := f.s.GetProject(ctx, f.projA); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("GetProject after delete: %v, want ErrNotFound", err)
			}
			if _, err := f.s.GetTask(ctx, f.a1); !errors.Is(err, store.ErrNotFound) {
				t.Errorf("GetTask of the project's task: %v, want ErrNotFound", err)
			}
			if _, err := f.s.GetTask(ctx, f.b1); err != nil {
				t.Errorf("other project's task: %v, want it kept", err)
			}
			if !slices.Equal(res.Projects, []int64{f.projA}) {
				t.Errorf("Projects = %v, want [%d]", res.Projects, f.projA)
			}

			var wantDeleted []int64
			if tt.deleteEntries {
				wantDeleted = entries
			}
			if !slices.Equal(res.DeletedEntries, wantDeleted) {
				t.Errorf("DeletedEntries = %v, want %v", res.DeletedEntries, wantDeleted)
			}
			for _, id := range entries {
				e, err := f.s.GetEntry(ctx, id)
				switch {
				case tt.deleteEntries && !errors.Is(err, store.ErrNotFound):
					t.Errorf("entry %d: %v, want ErrNotFound", id, err)
				case !tt.deleteEntries && (err != nil || e.ProjectID != nil):
					t.Errorf("entry %d = %+v, %v; want it kept, unscoped", id, e, err)
				}
			}

			applied := must(f.s.ListChangeOps(ctx, res.Changeset.ID))(t)[0].AppliedPayload
			var got struct {
				DeleteEntries bool `json:"delete_entries"`
				Deleted       struct {
					ID       int64   `json:"id"`
					Tasks    int     `json:"tasks"`
					EntryIDs []int64 `json:"entry_ids"`
				} `json:"deleted"`
			}
			if err := json.Unmarshal(applied, &got); err != nil || got.Deleted.ID != f.projA ||
				got.Deleted.Tasks == 0 || !slices.Equal(got.Deleted.EntryIDs, entries) || got.DeleteEntries != tt.deleteEntries {
				t.Errorf("applied payload %s, want the deleted project with its entries", applied)
			}
			if len(after.got) != 1 || !slices.Equal(after.got[0].DeletedEntries, wantDeleted) {
				t.Errorf("AfterApply got %+v, want the result once", after.got)
			}
		})
	}
}

// TestApplyManualDeleteProjectRunning checks that deleting a project with its
// entries is refused, changing nothing, while one of their runs is running.
func TestApplyManualDeleteProjectRunning(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	scopedEntries(t, f, f.projA, "running")
	before := dbSnapshot(t, f)

	_, err := NewApplier(f.s, nil).ApplyManual(ctx, DeleteProject{ProjectID: ID(f.projA), DeleteEntries: true})
	if !errors.Is(err, store.ErrRunInProgress) {
		t.Fatalf("err = %v, want ErrRunInProgress", err)
	}
	if after := dbSnapshot(t, f); after != before {
		t.Error("the database changed")
	}

	// Keeping the entries doesn't touch the runs, so it goes ahead.
	if _, err := NewApplier(f.s, nil).ApplyManual(ctx, DeleteProject{ProjectID: ID(f.projA)}); err != nil {
		t.Errorf("keeping the entries: %v, want it applied", err)
	}
}

func TestApplyManualDeleteProjectMissing(t *testing.T) {
	f := newFixture(t)
	_, err := NewApplier(f.s, nil).ApplyManual(context.Background(), DeleteProject{ProjectID: ID(99)})
	if got := fields(t, err); !slices.Equal(got, []string{"project_id"}) {
		t.Errorf("problems on %v, want project_id", got)
	}
}
