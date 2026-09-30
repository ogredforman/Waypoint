package store

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestExtractionRuns(t *testing.T) {
	s, _, c := newTestStore(t)
	ctx := context.Background()
	e1 := seedEntry(t, s, "first")
	e2 := seedEntry(t, s, "second")

	var r1, r2 domain.ExtractionRun
	inTx(t, s, func(tx *Tx) (err error) {
		// Status and results are ignored: a new run is always pending.
		if r1, err = tx.CreateExtractionRun(ctx, domain.ExtractionRun{
			EntryID: e1.ID, Provider: "mock", Model: "m", PromptVersion: "v1",
			Status: domain.RunSucceeded, RawOutput: json.RawMessage(`{}`),
		}); err != nil {
			return err
		}
		r2, err = tx.CreateExtractionRun(ctx, domain.ExtractionRun{EntryID: e2.ID, Provider: "mock"})
		return err
	})
	want := domain.ExtractionRun{
		ID: r1.ID, EntryID: e1.ID, Provider: "mock", Model: "m", PromptVersion: "v1",
		Status: domain.RunPending, CreatedAt: t0, UpdatedAt: t0,
	}
	if !reflect.DeepEqual(r1, want) {
		t.Errorf("CreateExtractionRun = %+v, want %+v", r1, want)
	}
	if n, err := s.CountActiveExtractionRuns(ctx, e1.ID); err != nil || n != 1 {
		t.Errorf("CountActiveExtractionRuns = %d, %v; want 1", n, err)
	}

	// Claims go oldest first.
	c.t = t0.Add(time.Second)
	var claimed domain.ExtractionRun
	inTx(t, s, func(tx *Tx) (err error) {
		claimed, err = tx.ClaimNextExtractionRun(ctx)
		return err
	})
	if claimed.ID != r1.ID || claimed.Status != domain.RunRunning || !claimed.UpdatedAt.Equal(c.t) {
		t.Errorf("first claim = %+v, want run %d running", claimed, r1.ID)
	}
	inTx(t, s, func(tx *Tx) (err error) {
		claimed, err = tx.ClaimNextExtractionRun(ctx)
		return err
	})
	if claimed.ID != r2.ID {
		t.Errorf("second claim = run %d, want %d", claimed.ID, r2.ID)
	}
	err := s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.ClaimNextExtractionRun(ctx)
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("claim with none pending = %v, want ErrNotFound", err)
	}

	snap := json.RawMessage(`{"projects":[]}`)
	inTx(t, s, func(tx *Tx) (err error) {
		_, err = tx.SetExtractionRunSnapshot(ctx, r1.ID, snap)
		return err
	})

	latency := int64(12)
	var done domain.ExtractionRun
	inTx(t, s, func(tx *Tx) (err error) {
		done, err = tx.FinishExtractionRun(ctx, r1.ID, RunOutcome{
			Status: domain.RunSucceeded, RawOutput: json.RawMessage(`{"ops":[]}`), LatencyMS: &latency,
		})
		return err
	})
	if done.Status != domain.RunSucceeded || string(done.InputSnapshot) != string(snap) ||
		string(done.RawOutput) != `{"ops":[]}` || done.LatencyMS == nil || *done.LatencyMS != 12 || done.Error != nil {
		t.Errorf("finished run = %+v", done)
	}
	if got, err := s.GetExtractionRun(ctx, r1.ID); err != nil || !reflect.DeepEqual(got, done) {
		t.Errorf("GetExtractionRun = %+v, %v; want %+v", got, err, done)
	}

	// Only a running run can finish.
	err = s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.FinishExtractionRun(ctx, r1.ID, RunOutcome{Status: domain.RunSucceeded})
		return err
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("finishing a finished run = %v, want ErrNotFound", err)
	}

	// r2 is still running: a restart puts it back to pending.
	var reset int
	inTx(t, s, func(tx *Tx) (err error) {
		reset, err = tx.ResetRunningExtractionRuns(ctx)
		return err
	})
	if reset != 1 {
		t.Errorf("reset %d runs, want 1", reset)
	}
	if got, err := s.GetExtractionRun(ctx, r2.ID); err != nil || got.Status != domain.RunPending {
		t.Errorf("reset run = %+v, %v; want pending", got, err)
	}

	runs, err := s.ListExtractionRunsByEntry(ctx, e1.ID)
	if err != nil || len(runs) != 1 || runs[0].ID != r1.ID {
		t.Errorf("ListExtractionRunsByEntry = %+v, %v", runs, err)
	}
	if n, err := s.CountActiveExtractionRuns(ctx, e1.ID); err != nil || n != 0 {
		t.Errorf("CountActiveExtractionRuns after finishing = %d, %v; want 0", n, err)
	}
	if _, err := s.GetExtractionRun(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetExtractionRun(999) = %v, want ErrNotFound", err)
	}
}

func TestFinishExtractionRunChecksOutcome(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	e := seedEntry(t, s, "body")
	var run domain.ExtractionRun
	inTx(t, s, func(tx *Tx) (err error) {
		if _, err = tx.CreateExtractionRun(ctx, domain.ExtractionRun{EntryID: e.ID, Provider: "mock"}); err != nil {
			return err
		}
		run, err = tx.ClaimNextExtractionRun(ctx)
		return err
	})
	msg := "boom"
	neg := int64(-1)
	for name, o := range map[string]RunOutcome{
		"pending":            {Status: domain.RunPending},
		"failed no error":    {Status: domain.RunFailed},
		"succeeded, error":   {Status: domain.RunSucceeded, Error: &msg},
		"invalid raw output": {Status: domain.RunSucceeded, RawOutput: json.RawMessage(`{oops`)},
		"negative latency":   {Status: domain.RunSucceeded, LatencyMS: &neg},
	} {
		err := s.InTx(ctx, func(tx *Tx) error {
			_, err := tx.FinishExtractionRun(ctx, run.ID, o)
			return err
		})
		if err == nil {
			t.Errorf("%s: FinishExtractionRun accepted %+v", name, o)
		}
	}
	var failed domain.ExtractionRun
	inTx(t, s, func(tx *Tx) (err error) {
		failed, err = tx.FinishExtractionRun(ctx, run.ID, RunOutcome{Status: domain.RunFailed, Error: &msg})
		return err
	})
	if failed.Status != domain.RunFailed || failed.Error == nil || *failed.Error != msg || failed.RawOutput != nil {
		t.Errorf("failed run = %+v", failed)
	}
}

func TestQuestions(t *testing.T) {
	s, sqlDB, c := newTestStore(t)
	ctx := context.Background()
	e := seedEntry(t, s, "body")
	other := seedEntry(t, s, "other")
	run := seedRun(t, sqlDB, e.ID)
	otherRun := seedRun(t, sqlDB, other.ID)

	var q1 domain.FollowUpQuestion
	inTx(t, s, func(tx *Tx) (err error) {
		if q1, err = tx.CreateQuestion(ctx, domain.FollowUpQuestion{
			EntryID: e.ID, ExtractionRunID: run, Text: "What's next?", Reason: "No next step", Status: domain.QuestionOpen,
		}); err != nil {
			return err
		}
		if _, err = tx.CreateQuestion(ctx, domain.FollowUpQuestion{
			EntryID: e.ID, ExtractionRunID: run, Text: "Blocked?", Status: domain.QuestionDismissed,
		}); err != nil {
			return err
		}
		_, err = tx.CreateQuestion(ctx, domain.FollowUpQuestion{
			EntryID: other.ID, ExtractionRunID: otherRun, Text: "Other?", Status: domain.QuestionOpen,
		})
		return err
	})
	want := domain.FollowUpQuestion{
		ID: q1.ID, EntryID: e.ID, ExtractionRunID: run, Text: "What's next?", Reason: "No next step",
		Status: domain.QuestionOpen, CreatedAt: t0, UpdatedAt: t0,
	}
	if q1 != want {
		t.Errorf("CreateQuestion = %+v, want %+v", q1, want)
	}
	err := s.InTx(ctx, func(tx *Tx) error {
		_, err := tx.CreateQuestion(ctx, domain.FollowUpQuestion{EntryID: e.ID, ExtractionRunID: run, Status: domain.QuestionOpen})
		return err
	})
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		t.Errorf("CreateQuestion without text = %v, want a validation error", err)
	}

	c.t = t0.Add(time.Minute)
	var n int
	inTx(t, s, func(tx *Tx) (err error) {
		n, err = tx.DismissOpenQuestionsByEntry(ctx, e.ID)
		return err
	})
	if n != 1 {
		t.Errorf("dismissed %d questions, want 1", n)
	}
	qs, err := s.ListQuestionsByEntry(ctx, e.ID)
	if err != nil || len(qs) != 2 {
		t.Fatalf("ListQuestionsByEntry = %+v, %v", qs, err)
	}
	if qs[0].Status != domain.QuestionDismissed || !qs[0].UpdatedAt.Equal(c.t) {
		t.Errorf("dismissed question = %+v", qs[0])
	}
	others, err := s.ListQuestionsByEntry(ctx, other.ID)
	if err != nil || len(others) != 1 || others[0].Status != domain.QuestionOpen {
		t.Errorf("other entry's questions = %+v, %v; want still open", others, err)
	}
}

func TestListChangesetsByEntry(t *testing.T) {
	s, sqlDB, _ := newTestStore(t)
	ctx := context.Background()
	e := seedEntry(t, s, "body")
	other := seedEntry(t, s, "other")
	run1 := seedRun(t, sqlDB, e.ID)
	run2 := seedRun(t, sqlDB, e.ID)
	otherRun := seedRun(t, sqlDB, other.ID)

	var ids []int64
	inTx(t, s, func(tx *Tx) error {
		for _, cs := range []domain.Changeset{
			{Source: domain.SourceExtraction, ExtractionRunID: &run1, Status: domain.ChangesetApplied},
			{Source: domain.SourceExtraction, ExtractionRunID: &otherRun, Status: domain.ChangesetPending},
			{Source: domain.SourceManual, Status: domain.ChangesetApplied},
			{Source: domain.SourceExtraction, ExtractionRunID: &run2, Status: domain.ChangesetPending},
		} {
			created, _, err := tx.CreateChangeset(ctx, cs, nil)
			if err != nil {
				return err
			}
			ids = append(ids, created.ID)
		}
		return nil
	})
	got, err := s.ListChangesetsByEntry(ctx, e.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != ids[0] || got[1].ID != ids[3] {
		t.Errorf("ListChangesetsByEntry = %+v, want changesets %d and %d", got, ids[0], ids[3])
	}
}

func TestInReadTxDoesNotBlockWriters(t *testing.T) {
	s, _, _ := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s)

	err := s.InReadTx(ctx, func(rtx *ReadTx) error {
		before, err := rtx.ListProjects(ctx)
		if err != nil {
			return err
		}
		// A write on another connection goes through while the read
		// transaction is open, and the read keeps its snapshot.
		if err := s.InTx(ctx, func(tx *Tx) error {
			_, err := tx.CreateProject(ctx, domain.Project{Title: "Second", Status: domain.ProjectParked})
			return err
		}); err != nil {
			return err
		}
		after, err := rtx.ListProjects(ctx)
		if err != nil {
			return err
		}
		if len(before) != 1 || len(after) != 1 {
			t.Errorf("read tx saw %d then %d projects, want 1 both times", len(before), len(after))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.ListProjects(ctx); err != nil || len(got) != 2 {
		t.Errorf("after the read tx: %d projects, %v; want 2", len(got), err)
	}
}
