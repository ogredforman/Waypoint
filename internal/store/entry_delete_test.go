package store

import (
	"context"
	"errors"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestDeleteEntryRemovesWhatCameFromIt(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	p := seedProject(t, s)

	// asker's question q1 is answered by doomed; doomed's question q2 is
	// answered by reply. kept is an unrelated entry.
	asker := seedEntry(t, s, "asker")
	askerRun := seedRun(t, sqlDB, asker.ID)
	exec(t, sqlDB, `INSERT INTO follow_up_questions (id, entry_id, extraction_run_id, text, status, created_at, updated_at)
		VALUES (1, ?, ?, 'q1', 'open', 0, 0)`, asker.ID, askerRun)
	var doomed domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		doomed, err = tx.CreateEntry(ctx, domain.Entry{Body: "doomed", ReplyToQuestionID: ptr(int64(1))})
		if err != nil {
			return err
		}
		_, err = tx.AnswerQuestion(ctx, 1, doomed.ID)
		return err
	})
	run := seedRun(t, sqlDB, doomed.ID)
	exec(t, sqlDB, `INSERT INTO follow_up_questions (id, entry_id, extraction_run_id, text, status, created_at, updated_at)
		VALUES (2, ?, ?, 'q2', 'open', 0, 0)`, doomed.ID, run)
	var reply domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		reply, err = tx.CreateEntry(ctx, domain.Entry{Body: "reply", ReplyToQuestionID: ptr(int64(2))})
		if err != nil {
			return err
		}
		_, err = tx.AnswerQuestion(ctx, 2, reply.ID)
		return err
	})
	kept := seedEntry(t, s, "kept")
	keptRun := seedRun(t, sqlDB, kept.ID)
	keptObs := linkTo(t, s, keptRun, kept.ID, domain.TargetProject, p.ID)

	// doomed's observation, an applied changeset with a pending split-off,
	// and a pending link on the split-off's op.
	obs := linkTo(t, s, run, doomed.ID, domain.TargetProject, p.ID)
	exec(t, sqlDB, `INSERT INTO changesets (id, source, extraction_run_id, status, created_at, updated_at)
		VALUES (10, 'extraction', ?, 'partially_applied', 0, 0)`, run)
	exec(t, sqlDB, `INSERT INTO changesets (id, source, extraction_run_id, parent_changeset_id, status, created_at, updated_at)
		VALUES (11, 'extraction', ?, 10, 'pending', 0, 0)`, run)
	exec(t, sqlDB, `INSERT INTO change_ops (id, changeset_id, position, op_type, payload, status, created_at, updated_at)
		VALUES (20, 10, 1, 'create_task', '{}', 'applied', 0, 0), (21, 11, 1, 'create_task', '{}', 'pending', 0, 0)`)
	exec(t, sqlDB, `INSERT INTO pending_observation_links (observation_id, change_op_id, target_type, confidence, created_at)
		VALUES (?, 21, 'task', 1, 0)`, obs.ID)
	// A manual changeset has no entry and stays.
	exec(t, sqlDB, `INSERT INTO changesets (id, source, status, created_at, updated_at) VALUES (12, 'manual', 'applied', 0, 0)`)

	// Rows created or resolved from doomed.
	task := seedTask(t, s, domain.Task{ProjectID: p.ID, Title: "From doomed", Status: domain.TaskTodo, OriginEntryID: &doomed.ID})
	var blocker domain.Blocker
	var idea domain.Idea
	inTx(t, s, func(tx *Tx) (err error) {
		blocker, err = tx.CreateBlocker(ctx, domain.Blocker{ProjectID: p.ID, Description: "Stuck", Status: domain.BlockerOpen, OriginEntryID: &doomed.ID})
		if err != nil {
			return err
		}
		idea, err = tx.CreateIdea(ctx, domain.Idea{Title: "Spark", Status: domain.IdeaParked, OriginEntryID: &doomed.ID})
		return err
	})
	exec(t, sqlDB, `UPDATE blockers SET status = 'resolved', resolved_entry_id = ? WHERE id = ?`, doomed.ID, blocker.ID)
	exec(t, sqlDB, `INSERT INTO briefings (id, project_id, source, generated_at, created_at, updated_at)
		VALUES (1, ?, 'heuristic', 0, 0, 0)`, p.ID)
	exec(t, sqlDB, `INSERT INTO briefing_recap_points (briefing_id, entry_id, position, kind, text)
		VALUES (1, ?, 1, 'note', 'doomed'), (1, ?, 2, 'note', 'kept')`, doomed.ID, kept.ID)

	var deleted domain.Entry
	inTx(t, s, func(tx *Tx) (err error) {
		deleted, err = tx.DeleteEntry(ctx, doomed.ID)
		return err
	})
	if deleted.ID != doomed.ID || deleted.Body != "doomed" {
		t.Errorf("DeleteEntry returned %+v, want the entry", deleted)
	}

	if _, err := s.GetEntry(ctx, doomed.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEntry after delete: %v, want ErrNotFound", err)
	}
	for _, c := range []struct {
		what  string
		query string
		want  int
	}{
		{"runs of the entry", "SELECT COUNT(*) FROM extraction_runs WHERE entry_id = ?", 0},
		{"observations of the entry", "SELECT COUNT(*) FROM observations WHERE entry_id = ?", 0},
		{"questions of the entry", "SELECT COUNT(*) FROM follow_up_questions WHERE entry_id = ?", 0},
		{"recap points of the entry", "SELECT COUNT(*) FROM briefing_recap_points WHERE entry_id = ?", 0},
	} {
		if n := count(t, sqlDB, c.query, doomed.ID); n != c.want {
			t.Errorf("%s: %d, want %d", c.what, n, c.want)
		}
	}
	for _, c := range []struct {
		what  string
		query string
		want  int
	}{
		{"extraction changesets", "SELECT COUNT(*) FROM changesets WHERE source = 'extraction'", 0},
		{"manual changesets", "SELECT COUNT(*) FROM changesets WHERE source = 'manual'", 1},
		{"change ops", "SELECT COUNT(*) FROM change_ops", 0},
		{"pending links", "SELECT COUNT(*) FROM pending_observation_links", 0},
		{"observation links", "SELECT COUNT(*) FROM observation_links", 1},
		{"recap points", "SELECT COUNT(*) FROM briefing_recap_points", 1},
		{"entries", "SELECT COUNT(*) FROM entries", 3},
	} {
		if n := count(t, sqlDB, c.query); n != c.want {
			t.Errorf("%s: %d, want %d", c.what, n, c.want)
		}
	}
	if links, err := s.ListLinksByObservation(ctx, keptObs.ID); err != nil || len(links) != 1 {
		t.Errorf("kept entry's links = %+v, %v; want them untouched", links, err)
	}

	if q, err := s.GetQuestion(ctx, 1); err != nil || q.Status != domain.QuestionDismissed || q.AnswerEntryID != nil {
		t.Errorf("question the entry answered = %+v, %v; want dismissed with no answer", q, err)
	}
	if got, err := s.GetEntry(ctx, reply.ID); err != nil || got.ReplyToQuestionID != nil || got.Body != "reply" {
		t.Errorf("reply = %+v, %v; want its text kept and no reply link", got, err)
	}
	if got, err := s.GetTask(ctx, task.ID); err != nil || got.OriginEntryID != nil || !got.UpdatedAt.Equal(task.UpdatedAt) {
		t.Errorf("task = %+v, %v; want it kept with no origin", got, err)
	}
	if got, err := s.GetBlocker(ctx, blocker.ID); err != nil || got.OriginEntryID != nil || got.ResolvedEntryID != nil || got.Status != domain.BlockerResolved {
		t.Errorf("blocker = %+v, %v; want it kept, resolved, with no entries", got, err)
	}
	if got, err := s.GetIdea(ctx, idea.ID); err != nil || got.OriginEntryID != nil {
		t.Errorf("idea = %+v, %v; want it kept with no origin", got, err)
	}
	if n := count(t, sqlDB, "SELECT COUNT(*) FROM pragma_foreign_key_check"); n != 0 {
		t.Errorf("%d foreign key violations", n)
	}
}

func TestDeleteEntryMissing(t *testing.T) {
	s, _, _ := newTestStore(t)
	err := s.InTx(context.Background(), func(tx *Tx) error {
		_, err := tx.DeleteEntry(context.Background(), 99)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteEntry(99) = %v, want ErrNotFound", err)
	}
}
