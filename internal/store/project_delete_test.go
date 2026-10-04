package store

import (
	"context"
	"errors"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestDeleteProjectRemovesWhatIsInIt(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	other := seedProject(t, s)

	var scoped, unscoped domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		if scoped, err = tx.CreateEntry(ctx, domain.Entry{Body: "about it", ProjectID: &p.ID}); err != nil {
			return err
		}
		unscoped, err = tx.CreateEntry(ctx, domain.Entry{Body: "about anything"})
		return err
	})

	parent := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Parent", Status: domain.TaskTodo})
	sub := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "Step", Status: domain.TaskTodo, ParentTaskID: &parent.ID})
	keptTask := seedTask(t, s, domain.Task{ProjectID: other.ID, Title: "Elsewhere", Status: domain.TaskTodo})
	var blocker domain.Blocker
	var idea, looseIdea domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		if err = tx.AddTaskDependency(ctx, sub.ID, parent.ID); err != nil {
			return err
		}
		if blocker, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, TaskID: &sub.ID, Description: "Stuck", Status: domain.BlockerOpen}); err != nil {
			return err
		}
		if idea, err = tx.CreateIdea(ctx, domain.Idea{ProjectID: &p.ID, Title: "Spark", Status: domain.IdeaParked}); err != nil {
			return err
		}
		looseIdea, err = tx.CreateIdea(ctx, domain.Idea{Title: "Loose", Status: domain.IdeaParked})
		return err
	})
	exec(t, sqlDB, `INSERT INTO milestones (id, project_id, title, status, position, created_at, updated_at)
		VALUES (5, ?, 'M1', 'open', 1, 0, 0)`, p.ID)
	exec(t, sqlDB, `INSERT INTO briefings (id, project_id, next_task_id, source, generated_at, created_at, updated_at)
		VALUES (1, ?, ?, 'heuristic', 0, 0, 0)`, p.ID, sub.ID)
	exec(t, sqlDB, `INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text)
		VALUES (1, ?, 1, 'note', 'about it')`, scoped.ID)

	run := seedRun(t, sqlDB, unscoped.ID)
	for _, l := range []struct {
		kind domain.TargetType
		id   int64
	}{
		{domain.TargetProject, p.ID}, {domain.TargetTask, sub.ID}, {domain.TargetBlocker, blocker.ID},
		{domain.TargetIdea, idea.ID}, {domain.TargetMilestone, 5},
	} {
		linkTo(t, s, run, unscoped.ID, l.kind, l.id)
	}
	keptObs := linkTo(t, s, run, unscoped.ID, domain.TargetTask, keptTask.ID)

	var deleted domain.Project
	var n ProjectContents
	inTx(t, s, func(tx *Tx) (err error) {
		deleted, n, err = tx.DeleteProject(ctx, p.ID)
		return err
	})
	if deleted.ID != p.ID || deleted.Title != p.Title {
		t.Errorf("DeleteProject returned %+v, want the project", deleted)
	}
	want := ProjectContents{Tasks: 2, Blockers: 1, Ideas: 1, Milestones: 1, Entries: 1}
	if n != want {
		t.Errorf("contents = %+v, want %+v", n, want)
	}

	if _, err := s.GetProject(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetProject after delete: %v, want ErrNotFound", err)
	}
	for _, c := range []struct {
		what  string
		query string
		want  int
	}{
		{"tasks", "SELECT COUNT(*) FROM tasks", 1},
		{"dependencies", "SELECT COUNT(*) FROM task_dependencies", 0},
		{"blockers", "SELECT COUNT(*) FROM blockers", 0},
		{"ideas", "SELECT COUNT(*) FROM ideas", 1},
		{"milestones", "SELECT COUNT(*) FROM milestones", 0},
		{"briefings", "SELECT COUNT(*) FROM briefings", 0},
		{"recap points", "SELECT COUNT(*) FROM briefing_recap_points", 0},
		{"observation links", "SELECT COUNT(*) FROM observation_links", 1},
		{"observations", "SELECT COUNT(*) FROM observations", 6},
		{"entries", "SELECT COUNT(*) FROM entries", 2},
	} {
		if got := count(t, sqlDB, c.query); got != c.want {
			t.Errorf("%s: %d, want %d", c.what, got, c.want)
		}
	}
	if links, err := s.ListLinksByObservation(ctx, keptObs.ID); err != nil || len(links) != 1 {
		t.Errorf("other project's links = %+v, %v; want them untouched", links, err)
	}
	if got, err := s.GetEntry(ctx, scoped.ID); err != nil || got.ProjectID != nil || got.Body != "about it" {
		t.Errorf("scoped entry = %+v, %v; want its text kept and no project", got, err)
	}
	if _, err := s.GetIdea(ctx, looseIdea.ID); err != nil {
		t.Errorf("idea without a project: %v, want it kept", err)
	}
	if _, err := s.GetProject(ctx, other.ID); err != nil {
		t.Errorf("other project: %v, want it kept", err)
	}
	if got := count(t, sqlDB, "SELECT COUNT(*) FROM pragma_foreign_key_check"); got != 0 {
		t.Errorf("%d foreign key violations", got)
	}
}

func TestDeleteProjectMissing(t *testing.T) {
	s, _, _ := newTestStore(t)
	err := s.InTx(context.Background(), func(tx *Tx) error {
		_, _, err := tx.DeleteProject(context.Background(), 99)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteProject(99) = %v, want ErrNotFound", err)
	}
}

func TestCountRunningProjectRuns(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)
	var e domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		e, err = tx.CreateEntry(ctx, domain.Entry{Body: "b", ProjectID: &p.ID})
		return err
	})
	run := seedRun(t, sqlDB, e.ID)
	if n, err := s.CountRunningProjectRuns(ctx, p.ID); err != nil || n != 0 {
		t.Fatalf("pending run: %d, %v; want 0", n, err)
	}
	exec(t, sqlDB, `UPDATE extraction_runs SET status = 'running' WHERE id = ?`, run)
	if n, err := s.CountRunningProjectRuns(ctx, p.ID); err != nil || n != 1 {
		t.Errorf("running run: %d, %v; want 1", n, err)
	}
}
