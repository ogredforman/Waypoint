package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestBriefings(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Write the briefer", Status: domain.TaskTodo})

	if _, err := s.GetBriefing(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetBriefing before any = %v, want ErrNotFound", err)
	}

	var first domain.Briefing
	inTx(t, s, func(tx *Tx) (err error) {
		first, err = tx.UpsertBriefing(ctx, domain.Briefing{
			ProjectID: p.ID, NextTaskID: &task.ID, NextReason: "It's next.", Recap: "Started.",
			Source: domain.BriefingHeuristic, GeneratedAt: t0,
		})
		return err
	})
	got, err := s.GetBriefing(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, first) {
		t.Errorf("GetBriefing = %+v, want %+v", got, first)
	}

	// A second upsert replaces the first in place.
	c.t = t0.Add(time.Hour)
	var second domain.Briefing
	inTx(t, s, func(tx *Tx) (err error) {
		second, err = tx.UpsertBriefing(ctx, domain.Briefing{
			ProjectID: p.ID, NextReason: "Nothing open.", Source: domain.BriefingHeuristic, GeneratedAt: c.t,
		})
		return err
	})
	if second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("upsert made a new row: %+v after %+v", second, first)
	}
	if second.NextTaskID != nil || second.NextReason != "Nothing open." || second.Recap != "" || !second.UpdatedAt.Equal(c.t) {
		t.Errorf("upserted briefing = %+v", second)
	}

	err = s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.UpsertBriefing(ctx, domain.Briefing{ProjectID: p.ID, Source: "guess"})
		return err
	})
	var verr *domain.ValidationError
	if !errors.As(err, &verr) {
		t.Errorf("invalid briefing error = %v, want a *domain.ValidationError", err)
	}
}

func TestListTaskDependenciesByProject(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	var other domain.Project
	inTx(t, s, func(tx *Tx) (err error) {
		other, err = tx.CreateProject(ctx, domain.Project{Title: "Other", Status: domain.ProjectParked})
		return err
	})
	a := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "A", Status: domain.TaskTodo})
	b := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "B", Status: domain.TaskTodo})
	x := seedTask(t, s, domain.Task{ProjectID: other.ID, Title: "X", Status: domain.TaskTodo})
	y := seedTask(t, s, domain.Task{ProjectID: other.ID, Title: "Y", Status: domain.TaskTodo})
	inTx(t, s, func(tx *Tx) error {
		if err := tx.AddTaskDependency(ctx, b.ID, a.ID); err != nil {
			return err
		}
		return tx.AddTaskDependency(ctx, y.ID, x.ID)
	})

	deps, err := s.ListTaskDependenciesByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].TaskID != b.ID || deps[0].BlockedByTaskID != a.ID {
		t.Errorf("dependencies = %+v, want only %d blocked by %d", deps, b.ID, a.ID)
	}
}

func TestListRecentEntriesForProject(t *testing.T) {
	s, sqlDB, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	var other domain.Project
	inTx(t, s, func(tx *Tx) (err error) {
		other, err = tx.CreateProject(ctx, domain.Project{Title: "Other", Status: domain.ProjectParked})
		return err
	})
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "A", Status: domain.TaskTodo})
	otherTask := seedTask(t, s, domain.Task{ProjectID: other.ID, Title: "X", Status: domain.TaskTodo})
	var blocker domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		blocker, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "stuck", Status: domain.BlockerOpen})
		return err
	})

	entry := func(projectID *int64) domain.Entry {
		t.Helper()
		c.t = c.t.Add(time.Minute)
		var e domain.Entry
		inTx(t, s, func(tx *Tx) (err error) {
			e, err = tx.CreateEntry(ctx, domain.Entry{Body: "words", ProjectID: projectID})
			return err
		})
		return e
	}
	scoped := entry(&p.ID)
	scopedElsewhere := entry(&other.ID)
	viaProject := entry(nil)
	linkTo(t, s, seedRun(t, sqlDB, viaProject.ID), viaProject.ID, domain.TargetProject, p.ID)
	viaTask := entry(nil)
	linkTo(t, s, seedRun(t, sqlDB, viaTask.ID), viaTask.ID, domain.TargetTask, task.ID)
	viaBlocker := entry(nil)
	linkTo(t, s, seedRun(t, sqlDB, viaBlocker.ID), viaBlocker.ID, domain.TargetBlocker, blocker.ID)
	aboutOther := entry(nil)
	linkTo(t, s, seedRun(t, sqlDB, aboutOther.ID), aboutOther.ID, domain.TargetTask, otherTask.ID)
	_ = entry(nil) // unlinked
	// Scoped elsewhere but linked here: the user's scope wins.
	linkTo(t, s, seedRun(t, sqlDB, scopedElsewhere.ID), scopedElsewhere.ID, domain.TargetProject, p.ID)

	ids := func(es []domain.Entry) []int64 {
		out := []int64{}
		for _, e := range es {
			out = append(out, e.ID)
		}
		return out
	}
	got, err := s.ListRecentEntriesForProject(ctx, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []int64{viaBlocker.ID, viaTask.ID, viaProject.ID, scoped.ID}
	if !reflect.DeepEqual(ids(got), want) {
		t.Errorf("entries = %v, want %v", ids(got), want)
	}

	got, err = s.ListRecentEntriesForProject(ctx, p.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(got), want[:2]) {
		t.Errorf("entries with limit 2 = %v, want %v", ids(got), want[:2])
	}
	if _, err := s.ListRecentEntriesForProject(ctx, p.ID, 0); err == nil {
		t.Error("limit 0 accepted")
	}

	got, err = s.ListEntriesBeforeForProject(ctx, p.ID, viaTask.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(got), want[2:]) {
		t.Errorf("entries before %d = %v, want %v", viaTask.ID, ids(got), want[2:])
	}
	got, err = s.ListEntriesBeforeForProject(ctx, p.ID, viaBlocker.ID+100, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids(got), want[:1]) {
		t.Errorf("entries before a later ID with limit 1 = %v, want %v", ids(got), want[:1])
	}
	if _, err := s.ListEntriesBeforeForProject(ctx, p.ID, viaTask.ID, 0); err == nil {
		t.Error("limit 0 accepted before an entry")
	}
}
