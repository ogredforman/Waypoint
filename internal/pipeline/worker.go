package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
	"github.com/ogredforman/waypoint/internal/store"
)

// EventType names an event the worker emits. The values are the SSE event
// names (SPEC §10.5).
type EventType string

const (
	// EventEntryProcessed: a run succeeded; the entry's observations and
	// questions are stored.
	EventEntryProcessed EventType = "entry.processed"
	// EventChangesetReady: a run proposed ops, stored as a pending changeset.
	EventChangesetReady EventType = "changeset.ready"
	// EventRunFailed: a run failed; the entry can be reprocessed.
	EventRunFailed EventType = "run.failed"
)

// Event tells listeners that a run finished. ChangesetID is set only for
// EventChangesetReady. Events carry IDs only, never entry text.
type Event struct {
	Type        EventType
	EntryID     int64
	RunID       int64
	ChangesetID int64
}

// Notifier receives the worker's events. Notify is called from the worker
// goroutine after the run's results are committed, and must not block.
type Notifier interface {
	Notify(Event)
}

// NotifierFunc adapts a function to a Notifier.
type NotifierFunc func(Event)

func (f NotifierFunc) Notify(e Event) { f(e) }

// AfterRun is told about each entry whose extraction run succeeded, after its
// results are committed, such as to refresh briefings. It runs on the worker
// goroutine, so the next run waits for it. Its context is never cancelled.
type AfterRun interface {
	EntryProcessed(ctx context.Context, e domain.Entry)
}

// ErrRunInProgress is returned when reprocessing an entry that already has a
// pending or running run, or deleting one whose run is running. It is
// store.ErrRunInProgress, so the changeset applier can return it too.
var ErrRunInProgress = store.ErrRunInProgress

// retryDelay is how long the worker waits after failing to claim a run
// before it tries again.
const retryDelay = 5 * time.Second

// Worker processes extraction runs in the background, one at a time (SPEC
// §7.1): snapshot, extract, validate, then store the observations, changeset,
// and questions and emit events. Pending runs in the database are its queue,
// so they survive a restart.
type Worker struct {
	store     *store.Store
	snapshots *SnapshotBuilder
	extractor extract.Extractor
	notifier  Notifier
	after     AfterRun
	logger    *slog.Logger
	wake      chan struct{}
}

// NewWorker returns a worker that runs extractor over entries. A nil notifier
// drops events; after may be nil.
func NewWorker(s *store.Store, snapshots *SnapshotBuilder, extractor extract.Extractor, notifier Notifier, after AfterRun, logger *slog.Logger) *Worker {
	if notifier == nil {
		notifier = NotifierFunc(func(Event) {})
	}
	return &Worker{
		store:     s,
		snapshots: snapshots,
		extractor: extractor,
		notifier:  notifier,
		after:     after,
		logger:    logger,
		wake:      make(chan struct{}, 1),
	}
}

// SaveEntry stores e and a pending run for it in one transaction, so an entry
// is never left without a run, and wakes the worker. If e replies to a
// follow-up question, the question is marked answered in the same
// transaction. It returns as soon as everything is stored; extraction happens
// in the background. A project or question that doesn't exist, or a question
// that is no longer open, is a *domain.ValidationError.
func (w *Worker) SaveEntry(ctx context.Context, e domain.Entry) (domain.Entry, domain.ExtractionRun, error) {
	var (
		entry domain.Entry
		run   domain.ExtractionRun
	)
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		if err := checkEntryRefs(ctx, tx, e); err != nil {
			return err
		}
		var err error
		if entry, err = tx.CreateEntry(ctx, e); err != nil {
			return err
		}
		if e.ReplyToQuestionID != nil {
			if _, err := tx.AnswerQuestion(ctx, *e.ReplyToQuestionID, entry.ID); err != nil {
				return err
			}
		}
		run, err = tx.CreateExtractionRun(ctx, w.newRun(entry.ID))
		return err
	})
	if err != nil {
		return domain.Entry{}, domain.ExtractionRun{}, fmt.Errorf("save entry: %w", err)
	}
	w.Wake()
	return entry, run, nil
}

// checkEntryRefs checks that the project and question e points at exist and
// that the question is still open, so a bad reference is the caller's
// mistake rather than a foreign key failure.
func checkEntryRefs(ctx context.Context, tx *store.Tx, e domain.Entry) error {
	var problems []domain.FieldError
	if e.ProjectID != nil {
		if _, err := tx.GetProject(ctx, *e.ProjectID); errors.Is(err, store.ErrNotFound) {
			problems = append(problems, domain.FieldError{Field: "project_id", Problem: "no such project"})
		} else if err != nil {
			return err
		}
	}
	if e.ReplyToQuestionID != nil {
		q, err := tx.GetQuestion(ctx, *e.ReplyToQuestionID)
		switch {
		case errors.Is(err, store.ErrNotFound):
			problems = append(problems, domain.FieldError{Field: "reply_to_question_id", Problem: "no such question"})
		case err != nil:
			return err
		case q.Status != domain.QuestionOpen:
			problems = append(problems, domain.FieldError{Field: "reply_to_question_id", Problem: "the question is no longer open"})
		}
	}
	if len(problems) > 0 {
		return &domain.ValidationError{Entity: "entry", Problems: problems}
	}
	return nil
}

// Reprocess queues a new run for entry entryID and wakes the worker. In the
// same transaction it rejects the entry's pending changesets and dismisses
// its open questions: the new run proposes afresh, and leaving the old
// proposal open would let the same ops be applied twice. The UI warns about
// this before calling it. It returns ErrRunInProgress if the entry already
// has a pending or running run, and store.ErrNotFound if there is no such
// entry. A run whose failure couldn't be recorded stays running, so its entry
// can't be reprocessed until the next start resets it.
func (w *Worker) Reprocess(ctx context.Context, entryID int64) (domain.ExtractionRun, error) {
	var run domain.ExtractionRun
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		if _, err := tx.GetEntry(ctx, entryID); err != nil {
			return err
		}
		active, err := tx.CountActiveExtractionRuns(ctx, entryID)
		if err != nil {
			return err
		}
		if active > 0 {
			return ErrRunInProgress
		}
		changesets, err := tx.ListChangesetsByEntry(ctx, entryID)
		if err != nil {
			return err
		}
		for _, cs := range changesets {
			if cs.Status != domain.ChangesetPending {
				continue
			}
			if _, err := changeset.RejectPending(ctx, tx, cs.ID); err != nil {
				return err
			}
		}
		if _, err := tx.DismissOpenQuestionsByEntry(ctx, entryID); err != nil {
			return err
		}
		run, err = tx.CreateExtractionRun(ctx, w.newRun(entryID))
		return err
	})
	if err != nil {
		return domain.ExtractionRun{}, fmt.Errorf("reprocess entry %d: %w", entryID, err)
	}
	w.Wake()
	return run, nil
}

// DeleteEntry deletes entry entryID with everything derived from it
// (store.Tx.DeleteEntry, ADR 0007). A pending run goes with it, and the
// worker can't claim it afterwards; while a run is running the entry can't
// be deleted (ErrRunInProgress), since the worker would store its results.
// It returns the deleted entry, or store.ErrNotFound.
func (w *Worker) DeleteEntry(ctx context.Context, entryID int64) (domain.Entry, error) {
	var e domain.Entry
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		running, err := tx.CountRunningExtractionRuns(ctx, entryID)
		if err != nil {
			return err
		}
		if running > 0 {
			return ErrRunInProgress
		}
		e, err = tx.DeleteEntry(ctx, entryID)
		return err
	})
	if err != nil {
		return domain.Entry{}, fmt.Errorf("delete entry %d: %w", entryID, err)
	}
	return e, nil
}

// prompted is implemented by extractors that use the extraction prompt, so
// each run records which version it ran with. The mock extractor uses none.
type prompted interface {
	PromptVersion() string
}

// modeled is implemented by extractors that call a model, so each run records
// which one.
type modeled interface {
	Model() string
}

func (w *Worker) newRun(entryID int64) domain.ExtractionRun {
	run := domain.ExtractionRun{EntryID: entryID, Provider: w.extractor.Name()}
	if p, ok := w.extractor.(prompted); ok {
		run.PromptVersion = p.PromptVersion()
	}
	if m, ok := w.extractor.(modeled); ok {
		run.Model = m.Model()
	}
	return run
}

// Wake tells the worker there may be a pending run. It never blocks.
func (w *Worker) Wake() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Run processes pending runs until ctx is cancelled. It first puts runs left
// running by an earlier process back to pending. A run interrupted by ctx
// stays running and is picked up again on the next start. Run returns an
// error only if that first reset fails.
func (w *Worker) Run(ctx context.Context) error {
	var reset int
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		var err error
		reset, err = tx.ResetRunningExtractionRuns(ctx)
		return err
	})
	if err != nil {
		return fmt.Errorf("extraction worker: %w", err)
	}
	if reset > 0 {
		w.logger.Info("resuming interrupted extraction runs", "count", reset)
	}

	for {
		run, err := w.claim(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case errors.Is(err, store.ErrNotFound):
			if !w.wait(ctx, nil) {
				return nil
			}
		case err != nil:
			w.logger.Error("claiming extraction run failed", "err", err)
			if !w.wait(ctx, time.After(retryDelay)) {
				return nil
			}
		default:
			w.process(ctx, run)
		}
	}
}

// wait blocks until the worker is woken, retry fires, or ctx is cancelled.
// It reports whether to keep going.
func (w *Worker) wait(ctx context.Context, retry <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-w.wake:
		return true
	case <-retry:
		return true
	}
}

func (w *Worker) claim(ctx context.Context) (domain.ExtractionRun, error) {
	var run domain.ExtractionRun
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		var err error
		run, err = tx.ClaimNextExtractionRun(ctx)
		return err
	})
	return run, err
}

// Stages of a run, for failure messages and logs. A failed run's error starts
// with its stage and ": ", which is what FailureReason reads.
const (
	stageLoad        = "load entry"
	stageSnapshot    = "snapshot"
	stageExtract     = "extract"
	stageTimeout     = "timeout"     // extract, timed out
	stageUnreachable = "unreachable" // extract, couldn't connect
	stageValidate    = "validate"
	stageRetried     = "validate (retried)"
	stageStore       = "store results"
)

// fromExtractor reports whether a stage's error came from the extractor or
// the validator, and so could quote the entry.
func fromExtractor(stage string) bool {
	switch stage {
	case stageExtract, stageTimeout, stageUnreachable, stageValidate, stageRetried:
		return true
	}
	return false
}

// extractStage names an extractor error's stage: a timeout and a failed
// connection get their own, so the UI can say what happened.
func extractStage(err error) string {
	var ne net.Error
	var op *net.OpError
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &ne) && ne.Timeout():
		return stageTimeout
	case errors.As(err, &op) && op.Op == "dial":
		return stageUnreachable
	}
	return stageExtract
}

// problems lists a validation error's problems as "field: problem" lines, to
// feed back to the extractor.
func problems(err error) []string {
	var ve *domain.ValidationError
	if !errors.As(err, &ve) {
		return []string{err.Error()}
	}
	out := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		out[i] = p.Field + ": " + p.Problem
	}
	return out
}

// FailureReason is why a run failed, in the terms the UI uses.
type FailureReason string

const (
	FailureTimeout     FailureReason = "timeout"     // the extractor took too long
	FailureUnreachable FailureReason = "unreachable" // the extractor couldn't connect to its model
	FailureInvalid     FailureReason = "invalid"     // the result failed validation, even on the retry
	FailureOther       FailureReason = "error"
)

// FailureOf reads a failed run's error, as fail wrote it, for its reason.
func FailureOf(msg string) FailureReason {
	stage, _, _ := strings.Cut(msg, ": ")
	switch stage {
	case stageTimeout:
		return FailureTimeout
	case stageUnreachable:
		return FailureUnreachable
	case stageValidate, stageRetried:
		return FailureInvalid
	}
	return FailureOther
}

// process takes one running run to succeeded or failed.
func (w *Worker) process(ctx context.Context, run domain.ExtractionRun) {
	entry, err := w.store.GetEntry(ctx, run.EntryID)
	if err != nil {
		w.fail(ctx, run, stageLoad, err, nil, nil)
		return
	}
	var replyTo *domain.FollowUpQuestion
	if id := entry.ReplyToQuestionID; id != nil {
		q, err := w.store.GetQuestion(ctx, *id)
		if err != nil {
			w.fail(ctx, run, stageLoad, err, nil, nil)
			return
		}
		replyTo = &q
	}

	snap, err := w.snapshots.Build(ctx, entry)
	if err == nil {
		err = w.saveSnapshot(ctx, run.ID, snap)
	}
	if err != nil {
		w.fail(ctx, run, stageSnapshot, err, nil, nil)
		return
	}

	settings, err := w.store.GetSettings(ctx)
	if err != nil {
		w.fail(ctx, run, stageLoad, err, nil, nil)
		return
	}

	in := extract.ExtractInput{
		Entry: entry, Snapshot: snap, ReplyTo: replyTo,
		MaxQuestions: settings.QuestionsPerEntry, PromptVersion: run.PromptVersion,
	}
	start := time.Now()
	res, err := w.extract(ctx, in)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		w.fail(ctx, run, extractStage(err), err, nil, &latency)
		return
	}

	// Validate first: it catches what can't be encoded, such as a NaN
	// confidence. The raw output of an invalid result is kept if it encodes.
	// An invalid result gets one retry, told what was wrong; an extractor
	// error doesn't, so a timeout can't double.
	if verr := extract.Validate(in, res); verr != nil {
		raw, _ := json.Marshal(res)
		w.logger.Debug("extraction result invalid, retrying", "run_id", run.ID, "entry_id", run.EntryID)
		in.Retry = &extract.Retry{Previous: raw, Problems: problems(verr)}
		res, err = w.extract(ctx, in)
		latency = time.Since(start).Milliseconds()
		if err != nil {
			w.fail(ctx, run, extractStage(err), err, raw, &latency)
			return
		}
		if err := extract.Validate(in, res); err != nil {
			raw, _ := json.Marshal(res)
			w.fail(ctx, run, stageRetried, err, raw, &latency)
			return
		}
	}
	raw, err := json.Marshal(res)
	if err != nil {
		w.fail(ctx, run, stageValidate, fmt.Errorf("encode output: %w", err), nil, &latency)
		return
	}

	// The raw output keeps every question; only as many as the settings
	// allow are stored, whatever the extractor was asked for. Validate has
	// already enforced the hard ceiling.
	if len(res.Questions) > settings.QuestionsPerEntry {
		res.Questions = res.Questions[:settings.QuestionsPerEntry]
	}

	csID, err := w.storeResult(ctx, run, res, raw, latency)
	if err != nil {
		w.fail(ctx, run, stageStore, err, raw, &latency)
		return
	}

	w.logger.Debug("extraction run succeeded", "run_id", run.ID, "entry_id", run.EntryID, "latency_ms", latency)
	w.notifier.Notify(Event{Type: EventEntryProcessed, EntryID: run.EntryID, RunID: run.ID})
	if csID != 0 {
		w.notifier.Notify(Event{Type: EventChangesetReady, EntryID: run.EntryID, RunID: run.ID, ChangesetID: csID})
	}
	if w.after != nil {
		// The run is committed: finish what follows it even if shutdown
		// has started.
		w.after.EntryProcessed(context.WithoutCancel(ctx), entry)
	}
}

// extract calls the extractor, turning a panic into an error so a buggy
// provider fails the run instead of the process.
func (w *Worker) extract(ctx context.Context, in extract.ExtractInput) (res extract.ExtractionResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			// The panic value isn't included: it could quote the entry.
			err = errors.New("extractor panicked")
		}
	}()
	return w.extractor.Extract(ctx, in)
}

func (w *Worker) saveSnapshot(ctx context.Context, runID int64, snap extract.StateSnapshot) error {
	b, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("encode snapshot: %w", err)
	}
	return w.store.InTx(ctx, func(tx *store.Tx) error {
		_, err := tx.SetExtractionRunSnapshot(ctx, runID, b)
		return err
	})
}

// storeResult stores a validated result and marks the run succeeded, all in
// one transaction. It returns the new changeset's ID, or 0 if the result
// proposed no ops: a changeset with no ops could never be resolved.
func (w *Worker) storeResult(ctx context.Context, run domain.ExtractionRun, res extract.ExtractionResult, raw json.RawMessage, latency int64) (int64, error) {
	var csID int64
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		// The changeset goes first: temp links name its ops by ID.
		var opIDs []int64 // by position - 1
		if len(res.Ops) > 0 {
			ops := make([]domain.ChangeOp, len(res.Ops))
			for i, d := range res.Ops {
				ops[i] = domain.ChangeOp{
					Position:   int64(i + 1),
					OpType:     d.OpType,
					Payload:    d.Payload,
					Confidence: d.Confidence,
					Status:     domain.OpPending,
				}
				if d.Rationale != "" {
					ops[i].Rationale = &d.Rationale
				}
			}
			cs, created, err := tx.CreateChangeset(ctx, domain.Changeset{
				Source:          domain.SourceExtraction,
				ExtractionRunID: &run.ID,
				Status:          domain.ChangesetPending,
			}, ops)
			if err != nil {
				return err
			}
			csID = cs.ID
			for _, op := range created {
				opIDs = append(opIDs, op.ID)
			}
		}

		for _, o := range res.Observations {
			obs, err := tx.CreateObservation(ctx, domain.Observation{
				EntryID:         run.EntryID,
				ExtractionRunID: run.ID,
				Kind:            o.Kind,
				Summary:         o.Summary,
				SpanStart:       o.SpanStart,
				SpanEnd:         o.SpanEnd,
			})
			if err != nil {
				return err
			}
			for _, l := range o.Links {
				// Validate checked that the confidence is set, and that a
				// temp ref names an op creating this kind of entity. The link
				// waits on that op until it is applied or rejected.
				if l.TargetID.IsTemp() {
					if _, err := tx.CreatePendingLink(ctx, domain.PendingObservationLink{
						ObservationID: obs.ID,
						ChangeOpID:    opIDs[l.TargetID.Temp-1],
						TargetType:    l.TargetType,
						Confidence:    *l.Confidence,
					}); err != nil {
						return err
					}
					continue
				}
				// Validate checked the target against the snapshot, but it may
				// have been deleted while the extractor ran. The observation
				// still stands; only the link to the gone row is dropped.
				exists, err := targetExists(ctx, tx, l.TargetType, l.TargetID.ID)
				if err != nil {
					return err
				}
				if !exists {
					continue
				}
				if _, err := tx.CreateObservationLink(ctx, domain.ObservationLink{
					ObservationID: obs.ID,
					TargetType:    l.TargetType,
					TargetID:      l.TargetID.ID,
					Confidence:    *l.Confidence,
				}); err != nil {
					return err
				}
			}
		}

		for _, q := range res.Questions {
			if _, err := tx.CreateQuestion(ctx, domain.FollowUpQuestion{
				EntryID:         run.EntryID,
				ExtractionRunID: run.ID,
				Text:            q.Text,
				Reason:          q.Reason,
				Status:          domain.QuestionOpen,
			}); err != nil {
				return err
			}
		}

		_, err := tx.FinishExtractionRun(ctx, run.ID, store.RunOutcome{
			Status:    domain.RunSucceeded,
			RawOutput: raw,
			LatencyMS: &latency,
		})
		return err
	})
	return csID, err
}

// targetExists reports whether the row a link points at still exists.
// Validate rules out milestones.
func targetExists(ctx context.Context, tx *store.Tx, kind domain.TargetType, id int64) (bool, error) {
	var err error
	switch kind {
	case domain.TargetProject:
		_, err = tx.GetProject(ctx, id)
	case domain.TargetTask:
		_, err = tx.GetTask(ctx, id)
	case domain.TargetBlocker:
		_, err = tx.GetBlocker(ctx, id)
	case domain.TargetIdea:
		_, err = tx.GetIdea(ctx, id)
	default:
		return false, fmt.Errorf("link to unsupported target type %q", kind)
	}
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// fail marks run failed with an error naming the stage, and emits
// EventRunFailed. If ctx was cancelled the run is left running instead, to be
// resumed on the next start. The error is stored on the run but only logged
// for internal stages: an extractor's error could quote the entry.
func (w *Worker) fail(ctx context.Context, run domain.ExtractionRun, stage string, cause error, raw json.RawMessage, latency *int64) {
	if ctx.Err() != nil {
		return
	}
	attrs := []any{"run_id", run.ID, "entry_id", run.EntryID, "stage", stage}
	if !fromExtractor(stage) {
		attrs = append(attrs, "err", cause)
	}
	w.logger.Warn("extraction run failed", attrs...)

	msg := stage + ": " + cause.Error()
	err := w.store.InTx(ctx, func(tx *store.Tx) error {
		_, err := tx.FinishExtractionRun(ctx, run.ID, store.RunOutcome{
			Status:    domain.RunFailed,
			RawOutput: raw,
			Error:     &msg,
			LatencyMS: latency,
		})
		return err
	})
	if err != nil {
		// The run stays running until the next start resets it.
		w.logger.Error("recording failed extraction run", "run_id", run.ID, "err", err)
		return
	}
	w.notifier.Notify(Event{Type: EventRunFailed, EntryID: run.EntryID, RunID: run.ID})
}
