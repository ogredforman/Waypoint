package store

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

// linkTo inserts an observation on a fresh entry and links it to target.
func linkTo(t *testing.T, s *Store, run int64, entryID int64, targetType domain.TargetType, targetID int64) domain.Observation {
	t.Helper()
	ctx := context.Background()
	var o domain.Observation
	inTx(t, s, func(tx *Tx) (err error) {
		if o, err = tx.CreateObservation(ctx, domain.Observation{
			EntryID: entryID, ExtractionRunID: run, Kind: domain.ObservationNote, Summary: "note", SpanStart: 0, SpanEnd: 1,
		}); err != nil {
			return err
		}
		_, err = tx.CreateObservationLink(ctx, domain.ObservationLink{
			ObservationID: o.ID, TargetType: targetType, TargetID: targetID, Confidence: 1,
		})
		return err
	})
	return o
}

func TestDeleteTaskCleansUpReferences(t *testing.T) {
	s, sqlDB, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	e := seedEntry(t, s, "x")
	run := seedRun(t, sqlDB, e.ID)

	doomed := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Doomed", Status: domain.TaskTodo, OriginEntryID: &e.ID})
	child := seedTask(t, s, domain.Task{ProjectID: p.ID, ParentTaskID: &doomed.ID, Title: "Child", Status: domain.TaskTodo})
	before := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Before", Status: domain.TaskTodo})
	after := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "After", Status: domain.TaskTodo})
	exec(t, sqlDB, `INSERT INTO task_dependencies (task_id, blocked_by_task_id, created_at) VALUES (?, ?, 0), (?, ?, 0), (?, ?, 0)`,
		doomed.ID, before.ID, after.ID, doomed.ID, after.ID, before.ID)
	var blocker domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		blocker, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, TaskID: &doomed.ID, Description: "Stuck", Status: domain.BlockerOpen})
		return err
	})
	exec(t, sqlDB, `INSERT INTO briefings (project_id, next_task_id, source, generated_at, created_at, updated_at)
		VALUES (?, ?, 'heuristic', 0, 0, 0)`, p.ID, doomed.ID)
	obs := linkTo(t, s, run, e.ID, domain.TargetTask, doomed.ID)
	kept := linkTo(t, s, run, e.ID, domain.TargetTask, child.ID)

	c.t = t0.Add(time.Hour)
	var deleted domain.Task
	inTx(t, s, func(tx *Tx) (err error) {
		deleted, err = tx.DeleteTask(ctx, doomed.ID)
		return err
	})
	if !reflect.DeepEqual(deleted, doomed) {
		t.Errorf("DeleteTask returned %+v, want the task as it was: %+v", deleted, doomed)
	}

	if _, err := s.GetTask(ctx, doomed.ID); err == nil {
		t.Error("the task still exists")
	}
	if got, err := s.GetTask(ctx, child.ID); err != nil || got.ParentTaskID != nil || !got.UpdatedAt.Equal(c.t) {
		t.Errorf("subtask = %+v, %v; want it kept as a top-level task, updated now", got, err)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM task_dependencies"); n != 1 {
		t.Errorf("%d dependencies left, want only the one not involving the task", n)
	}
	if got, err := s.GetBlocker(ctx, blocker.ID); err != nil || got.TaskID != nil || got.ProjectID != p.ID || !got.UpdatedAt.Equal(c.t) {
		t.Errorf("blocker = %+v, %v; want it kept on the project without a task, updated now", got, err)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM briefings WHERE project_id = ? AND next_task_id IS NULL", p.ID); n != 1 {
		t.Error("the briefing still points at the task")
	}
	if links, err := s.ListLinksByTarget(ctx, domain.TargetTask, doomed.ID); err != nil || len(links) != 0 {
		t.Errorf("links to the task = %+v, %v; want none", links, err)
	}
	if _, err := s.GetObservation(ctx, obs.ID); err != nil {
		t.Errorf("the linked observation was removed: %v", err)
	}
	if links, err := s.ListLinksByObservation(ctx, kept.ID); err != nil || len(links) != 1 {
		t.Errorf("links to other tasks = %+v, %v; want them kept", links, err)
	}
	if _, err := s.GetEntry(ctx, e.ID); err != nil {
		t.Errorf("the origin entry was removed: %v", err)
	}
}

func TestDeleteTaskRollsBackWithTransaction(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	parent := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Parent", Status: domain.TaskTodo})
	seedTask(t, s, domain.Task{ProjectID: p.ID, ParentTaskID: &parent.ID, Title: "Child", Status: domain.TaskTodo})

	err := s.InTx(ctx, func(tx *Tx) error {
		if _, err := tx.DeleteTask(ctx, parent.ID); err != nil {
			return err
		}
		_, err := tx.DeleteTask(ctx, 999) // fails, so the whole transaction rolls back
		return err
	})
	if err == nil {
		t.Fatal("deleting a missing task succeeded")
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM tasks WHERE parent_task_id = ?", parent.ID); n != 1 {
		t.Error("the subtask was detached even though the transaction rolled back")
	}
	if _, err := s.GetTask(ctx, parent.ID); err != nil {
		t.Errorf("the task is gone after rollback: %v", err)
	}
}

func TestDeleteBlockerAndIdea(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	e := seedEntry(t, s, "x")
	run := seedRun(t, sqlDB, e.ID)

	var b domain.Blocker
	var i domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		if b, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "Stuck", Status: domain.BlockerOpen}); err != nil {
			return err
		}
		i, err = tx.CreateIdea(ctx, domain.Idea{Title: "Spark", Status: domain.IdeaParked})
		return err
	})
	// Same IDs across target types: deleting one must not touch the other's links.
	linkTo(t, s, run, e.ID, domain.TargetBlocker, b.ID)
	linkTo(t, s, run, e.ID, domain.TargetIdea, i.ID)
	linkTo(t, s, run, e.ID, domain.TargetProject, p.ID)

	var gotB domain.Blocker
	inTx(t, s, func(tx *Tx) (err error) {
		gotB, err = tx.DeleteBlocker(ctx, b.ID)
		return err
	})
	if !reflect.DeepEqual(gotB, b) {
		t.Errorf("DeleteBlocker returned %+v, want %+v", gotB, b)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM observation_links"); n != 2 {
		t.Errorf("%d links after deleting the blocker, want 2", n)
	}

	var gotI domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		gotI, err = tx.DeleteIdea(ctx, i.ID)
		return err
	})
	if !reflect.DeepEqual(gotI, i) {
		t.Errorf("DeleteIdea returned %+v, want %+v", gotI, i)
	}
	if links, err := s.ListLinksByTarget(ctx, domain.TargetProject, p.ID); err != nil || len(links) != 1 {
		t.Errorf("project links = %+v, %v; want the one link kept", links, err)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM blockers") + count(t, sqlDB, "SELECT COUNT(*) FROM ideas"); n != 0 {
		t.Errorf("%d blockers and ideas left, want 0", n)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM observations"); n != 3 {
		t.Errorf("%d observations left, want all 3 kept", n)
	}
}
