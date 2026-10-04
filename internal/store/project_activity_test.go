package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestProjectActivityFollowsWhatHappensInIt(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	quiet := seedProject(t, s)

	activity := func(id int64) time.Time {
		t.Helper()
		all, err := s.ProjectActivity(ctx)
		if err != nil {
			t.Fatal(err)
		}
		one, err := s.GetProjectActivity(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if !all[id].Equal(one) {
			t.Fatalf("project %d: list says %v, get says %v", id, all[id], one)
		}
		return one
	}

	if got := activity(p.ID); !got.Equal(t0) {
		t.Fatalf("new project: activity %v, want its creation %v", got, t0)
	}

	c.t = t0.Add(time.Hour)
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Write the spec", Status: domain.TaskTodo})
	if got := activity(p.ID); !got.Equal(c.t) {
		t.Fatalf("after a task: activity %v, want %v", got, c.t)
	}

	c.t = t0.Add(2 * time.Hour)
	inTx(t, s, func(tx *Tx) error {
		task.Status = domain.TaskDone
		_, err := tx.UpdateTask(ctx, task)
		return err
	})
	if got := activity(p.ID); !got.Equal(c.t) {
		t.Fatalf("after a task change: activity %v, want %v", got, c.t)
	}

	c.t = t0.Add(3 * time.Hour)
	inTx(t, s, func(tx *Tx) error {
		_, err := tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "Waiting on review", Status: domain.BlockerOpen})
		return err
	})
	if got := activity(p.ID); !got.Equal(c.t) {
		t.Fatalf("after a blocker: activity %v, want %v", got, c.t)
	}

	c.t = t0.Add(4 * time.Hour)
	inTx(t, s, func(tx *Tx) error {
		_, err := tx.CreateEntry(ctx, domain.Entry{Body: "Spent the evening on it", ProjectID: &p.ID})
		return err
	})
	if got := activity(p.ID); !got.Equal(c.t) {
		t.Fatalf("after an entry: activity %v, want %v", got, c.t)
	}

	if got := activity(quiet.ID); !got.Equal(t0) {
		t.Fatalf("untouched project: activity %v, want %v", got, t0)
	}
}

func TestGetProjectActivityMissing(t *testing.T) {
	s, _, _ := newTestStore(t)
	if _, err := s.GetProjectActivity(context.Background(), 99); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
