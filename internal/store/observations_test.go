package store

import (
	"context"
	"slices"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestListProjectObservationsBefore(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Write tests", Status: domain.TaskTodo})
	var other domain.Project
	var blocker domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		if other, err = tx.CreateProject(ctx, domain.Project{Title: "Garden", Status: domain.ProjectParked}); err != nil {
			return err
		}
		blocker, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "Stuck", Status: domain.BlockerOpen})
		return err
	})
	entry := func() (domain.Entry, int64) {
		e := seedEntry(t, s, "some words")
		return e, seedRun(t, sqlDB, e.ID)
	}

	e1, r1 := entry()
	onProject := linkTo(t, s, r1, e1.ID, domain.TargetProject, p.ID)

	// Linked to the task and the project: listed once.
	e2, r2 := entry()
	onTask := linkTo(t, s, r2, e2.ID, domain.TargetTask, task.ID)
	inTx(t, s, func(tx *Tx) error {
		_, err := tx.CreateObservationLink(ctx, domain.ObservationLink{
			ObservationID: onTask.ID, TargetType: domain.TargetProject, TargetID: p.ID, Confidence: 0.5,
		})
		return err
	})

	e3, r3 := entry()
	onBlocker := linkTo(t, s, r3, e3.ID, domain.TargetBlocker, blocker.ID)

	// About another project only.
	e4, r4 := entry()
	linkTo(t, s, r4, e4.ID, domain.TargetProject, other.ID)

	// Reprocessed: only the latest run's observation counts.
	e5, r5 := entry()
	linkTo(t, s, r5, e5.ID, domain.TargetProject, p.ID)
	reread := linkTo(t, s, seedRun(t, sqlDB, e5.ID), e5.ID, domain.TargetProject, p.ID)

	// Reprocessed, and the latest run no longer mentions the project.
	e6, r6 := entry()
	linkTo(t, s, r6, e6.ID, domain.TargetProject, p.ID)
	linkTo(t, s, seedRun(t, sqlDB, e6.ID), e6.ID, domain.TargetProject, other.ID)

	ids := func(obs []ProjectObservation) []int64 {
		out := []int64{}
		for _, o := range obs {
			out = append(out, o.Observation.ID)
		}
		return out
	}
	list := func(before int64, limit int) []ProjectObservation {
		t.Helper()
		obs, err := s.ListProjectObservationsBefore(ctx, p.ID, before, limit)
		if err != nil {
			t.Fatal(err)
		}
		return obs
	}

	all := list(e6.ID+1, 10)
	if want := []int64{reread.ID, onBlocker.ID, onTask.ID, onProject.ID}; !slices.Equal(ids(all), want) {
		t.Fatalf("observations = %v, want %v (newest entry first)", ids(all), want)
	}
	if all[2].Confidence != 1 || all[2].EntryBody != "some words" || all[2].Observation.EntryID != e2.ID {
		t.Errorf("task observation = %+v, want the strongest confidence and its entry", all[2])
	}

	// Pages hold whole entries.
	if got, want := ids(list(e6.ID+1, 2)), []int64{reread.ID, onBlocker.ID}; !slices.Equal(got, want) {
		t.Errorf("first page = %v, want %v", got, want)
	}
	if got, want := ids(list(e3.ID, 2)), []int64{onTask.ID, onProject.ID}; !slices.Equal(got, want) {
		t.Errorf("second page = %v, want %v", got, want)
	}
	if got := list(e1.ID, 2); len(got) != 0 {
		t.Errorf("past the oldest = %v, want none", ids(got))
	}
	if _, err := s.ListProjectObservationsBefore(ctx, p.ID, e6.ID, 0); err == nil {
		t.Error("limit 0: want an error")
	}
}
