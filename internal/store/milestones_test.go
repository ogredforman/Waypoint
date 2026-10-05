package store

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestMilestones(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	other := seedProject(t, s)

	create := func(projectID int64, title string) domain.Milestone {
		t.Helper()
		var m domain.Milestone
		inTx(t, s, func(tx *Tx) (err error) {
			// Position is assigned by the store: each project's milestones append at the end.
			m, err = tx.CreateMilestone(ctx, domain.Milestone{
				ProjectID: projectID, Title: title, Description: "d", Status: domain.MilestoneOpen, Position: 99,
			})
			return err
		})
		return m
	}
	first := create(p.ID, "MVP")
	elsewhere := create(other.ID, "Unrelated")
	second := create(p.ID, "Beta")
	if first.Position != 1 || second.Position != 2 || elsewhere.Position != 1 {
		t.Errorf("positions = %d, %d (other project %d), want 1, 2 (1)", first.Position, second.Position, elsewhere.Position)
	}
	want := domain.Milestone{
		ID: second.ID, ProjectID: p.ID, Title: "Beta", Description: "d", Status: domain.MilestoneOpen,
		Position: 2, CreatedAt: t0, UpdatedAt: t0,
	}
	if !reflect.DeepEqual(second, want) {
		t.Errorf("CreateMilestone = %+v, want %+v", second, want)
	}
	if got, err := s.GetMilestone(ctx, second.ID); err != nil || !reflect.DeepEqual(got, second) {
		t.Errorf("GetMilestone = %+v, %v; want %+v", got, err, second)
	}

	c.t = t0.Add(time.Minute)
	edit := second
	edit.Title, edit.Description, edit.Status, edit.Position = "Beta 1", "", domain.MilestoneDone, 0
	edit.ProjectID = other.ID // ignored: fixed at creation
	var updated domain.Milestone
	inTx(t, s, func(tx *Tx) (err error) {
		updated, err = tx.UpdateMilestone(ctx, edit)
		return err
	})
	want.Title, want.Description, want.Status, want.Position, want.UpdatedAt = "Beta 1", "", domain.MilestoneDone, 0, c.t
	if !reflect.DeepEqual(updated, want) {
		t.Errorf("UpdateMilestone = %+v, want %+v", updated, want)
	}

	list, err := s.ListMilestonesByProject(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("ListMilestonesByProject = %+v, want milestones %d then %d", list, second.ID, first.ID)
	}

	err = s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.CreateMilestone(ctx, domain.Milestone{ProjectID: p.ID, Title: " ", Status: domain.MilestoneOpen})
		return err
	})
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("CreateMilestone with a blank title: %v, want a ValidationError", err)
	}
	inTx(t, s, func(tx *Tx) error {
		if _, err := tx.GetMilestone(ctx, 999); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetMilestone(999): %v, want ErrNotFound", err)
		}
		if _, err := tx.UpdateMilestone(ctx, domain.Milestone{ID: 999, ProjectID: p.ID, Title: "x", Status: domain.MilestoneOpen}); !errors.Is(err, ErrNotFound) {
			t.Errorf("UpdateMilestone(999): %v, want ErrNotFound", err)
		}
		if _, err := tx.DeleteMilestone(ctx, 999); !errors.Is(err, ErrNotFound) {
			t.Errorf("DeleteMilestone(999): %v, want ErrNotFound", err)
		}
		return nil
	})
}

func TestDeleteMilestoneKeepsItsTasks(t *testing.T) {
	s, sqlDB, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	e := seedEntry(t, s, "x")
	run := seedRun(t, sqlDB, e.ID)

	var doomed, kept domain.Milestone
	inTx(t, s, func(tx *Tx) (err error) {
		if doomed, err = tx.CreateMilestone(ctx, domain.Milestone{ProjectID: p.ID, Title: "Doomed", Status: domain.MilestoneOpen}); err != nil {
			return err
		}
		kept, err = tx.CreateMilestone(ctx, domain.Milestone{ProjectID: p.ID, Title: "Kept", Status: domain.MilestoneOpen})
		return err
	})
	inDoomed := seedTask(t, s, domain.Task{ProjectID: p.ID, MilestoneID: &doomed.ID, Title: "In doomed", Status: domain.TaskTodo})
	inKept := seedTask(t, s, domain.Task{ProjectID: p.ID, MilestoneID: &kept.ID, Title: "In kept", Status: domain.TaskTodo})
	linkTo(t, s, run, e.ID, domain.TargetMilestone, doomed.ID)
	linkTo(t, s, run, e.ID, domain.TargetMilestone, kept.ID)

	c.t = t0.Add(time.Hour)
	var deleted domain.Milestone
	inTx(t, s, func(tx *Tx) (err error) {
		deleted, err = tx.DeleteMilestone(ctx, doomed.ID)
		return err
	})
	if !reflect.DeepEqual(deleted, doomed) {
		t.Errorf("DeleteMilestone returned %+v, want the milestone as it was: %+v", deleted, doomed)
	}
	if _, err := s.GetMilestone(ctx, doomed.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetMilestone after delete: %v, want ErrNotFound", err)
	}
	if got, err := s.GetTask(ctx, inDoomed.ID); err != nil || got.MilestoneID != nil || !got.UpdatedAt.Equal(c.t) {
		t.Errorf("task in the milestone = %+v, %v; want it kept with no milestone, updated now", got, err)
	}
	if got, err := s.GetTask(ctx, inKept.ID); err != nil || got.MilestoneID == nil || *got.MilestoneID != kept.ID {
		t.Errorf("task in another milestone = %+v, %v; want it untouched", got, err)
	}
	if links, err := s.ListLinksByTarget(ctx, domain.TargetMilestone, doomed.ID); err != nil || len(links) != 0 {
		t.Errorf("links to the milestone = %+v, %v; want none", links, err)
	}
	if links, err := s.ListLinksByTarget(ctx, domain.TargetMilestone, kept.ID); err != nil || len(links) != 1 {
		t.Errorf("links to the other milestone = %+v, %v; want one", links, err)
	}
}
