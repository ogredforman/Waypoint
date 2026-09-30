package mock

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// Name is what the mock reports as its provider.
const Name = "mock"

// Confidences the mock reports. A project guessed from the entry is less
// certain than one the user chose, and a guess with no clues at all is barely
// one; the review UI lets the user pick another project either way.
const (
	sureConfidence    = 1.0
	onlyProjectGuess  = 0.9
	cluedProjectGuess = 0.7
	blindProjectGuess = 0.3
)

// Project clues: a line naming one of a project's tasks or blockers counts
// more than a word of its title appearing in the entry.
const (
	titleWordClue = 1
	matchClue     = 2
)

const (
	matchThreshold   = 0.5 // lowest similarity that counts as a fuzzy match
	containmentScore = 0.75
	stalledAfter     = 3 * 24 * time.Hour
)

// Extractor is the deterministic, rule-based extractor from SPEC §10.4. Line
// prefixes ("todo:", "done:", ...) map to observations and ops; everything
// else is a note. The same input always gives the same result.
type Extractor struct {
	// Latency is an artificial delay before each result, to exercise the
	// async UI. Zero means none.
	Latency time.Duration
}

// New returns a mock extractor that waits latency before each result.
func New(latency time.Duration) *Extractor {
	return &Extractor{Latency: latency}
}

func (*Extractor) Name() string { return Name }

// Extract turns the entry's lines into a result. It returns early with the
// context's error if ctx is done during the artificial delay.
func (e *Extractor) Extract(ctx context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	if err := ctx.Err(); err != nil {
		return extract.ExtractionResult{}, err
	}
	if e.Latency > 0 {
		t := time.NewTimer(e.Latency)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return extract.ExtractionResult{}, ctx.Err()
		case <-t.C:
		}
	}
	b := newBuilder(in)
	return b.build()
}

// prefix is the kind of an entry line.
type prefix string

const (
	prefixDone      prefix = "done"
	prefixDoing     prefix = "doing"
	prefixTodo      prefix = "todo"
	prefixBlocked   prefix = "blocked"
	prefixUnblocked prefix = "unblocked"
	prefixIdea      prefix = "idea"
	prefixFeel      prefix = "feel"
	prefixDecided   prefix = "decided"
	prefixNone      prefix = ""
)

// known reports whether p is one of the prefixes in SPEC §10.4.
func (p prefix) known() bool {
	switch p {
	case prefixDone, prefixDoing, prefixTodo, prefixBlocked, prefixUnblocked,
		prefixIdea, prefixFeel, prefixDecided:
		return true
	}
	return false
}

// line is one non-blank line of the entry. start and end are byte offsets of
// its text in the body, without indentation, bullet, or line ending.
type line struct {
	prefix     prefix
	text       string // after the prefix; the whole line for prefixNone
	start, end int64
}

// splitLines returns the entry's non-blank lines. A leading "- " or "* "
// bullet is dropped; prefixes match case-insensitively. A prefix with no text
// after it makes the line a plain note.
func splitLines(body string) []line {
	var out []line
	for off := 0; off <= len(body); {
		n := strings.IndexByte(body[off:], '\n')
		if n < 0 {
			n = len(body) - off
		}
		raw := body[off : off+n]
		start := off
		off += n + 1

		trimmed := strings.TrimLeftFunc(raw, unicode.IsSpace)
		for _, bullet := range []string{"- ", "* "} {
			if rest, ok := strings.CutPrefix(trimmed, bullet); ok {
				trimmed = strings.TrimLeftFunc(rest, unicode.IsSpace)
				break
			}
		}
		start += len(raw) - len(trimmed)
		trimmed = strings.TrimRightFunc(trimmed, unicode.IsSpace) // also drops a CRLF's \r
		if trimmed == "" {
			continue
		}
		l := line{prefix: prefixNone, text: trimmed, start: int64(start), end: int64(start + len(trimmed))}
		if head, rest, ok := strings.Cut(trimmed, ":"); ok {
			p := prefix(strings.ToLower(head))
			if text := strings.TrimSpace(rest); p.known() && text != "" {
				l.prefix, l.text = p, text
			}
		}
		out = append(out, l)
	}
	return out
}

// taskCandidate is a task a done:/doing: line may name: an open task from the
// snapshot, or one created earlier in this entry.
type taskCandidate struct {
	ref       changeset.Ref
	projectID int64
	title     string
	status    domain.TaskStatus
}

type blockerCandidate struct {
	id          int64
	projectID   int64
	taskID      *int64
	description string
	resolved    bool // by an earlier line of this entry
}

// builder accumulates one result.
type builder struct {
	in       extract.ExtractInput
	lines    []line
	tasks    []*taskCandidate
	blockers []*blockerCandidate

	project        int64 // 0: none to use
	projectGuessed bool
	projectConf    float64

	touched map[int64]bool // snapshot tasks a line of this entry named
	res     extract.ExtractionResult
}

func newBuilder(in extract.ExtractInput) *builder {
	b := &builder{in: in, lines: splitLines(in.Entry.Body), touched: map[int64]bool{}}
	for _, p := range in.Snapshot.Projects {
		for _, t := range p.Tasks {
			b.tasks = append(b.tasks, &taskCandidate{ref: changeset.ID(t.ID), projectID: p.ID, title: t.Title, status: t.Status})
		}
		for _, bl := range p.Blockers {
			b.blockers = append(b.blockers, &blockerCandidate{id: bl.ID, projectID: p.ID, taskID: bl.TaskID, description: bl.Description})
		}
	}
	// Ties in matching go to the lowest ID.
	slices.SortStableFunc(b.tasks, func(x, y *taskCandidate) int { return cmp.Compare(x.ref.ID, y.ref.ID) })
	slices.SortStableFunc(b.blockers, func(x, y *blockerCandidate) int { return cmp.Compare(x.id, y.id) })
	return b
}

func (b *builder) build() (extract.ExtractionResult, error) {
	b.res = extract.ExtractionResult{
		Observations: []extract.ObservationDraft{},
		Ops:          []extract.ChangeOpDraft{},
		Questions:    []extract.QuestionDraft{},
	}
	b.pickProject()
	for _, l := range b.lines {
		if err := b.line(l); err != nil {
			return extract.ExtractionResult{}, err
		}
	}
	b.questions()
	return b.res, nil
}

// pickProject decides which project new tasks, blockers, and ideas go into.
// The entry's own project wins. Otherwise the mock guesses: the only project
// in the snapshot, or the one with the most clues (title words in the entry,
// and lines naming its tasks or blockers), or failing any clue the first one.
func (b *builder) pickProject() {
	if id := b.in.Entry.ProjectID; id != nil {
		b.project, b.projectConf = *id, sureConfidence
		return
	}
	projects := b.in.Snapshot.Projects
	switch len(projects) {
	case 0:
		return
	case 1:
		b.project, b.projectGuessed, b.projectConf = projects[0].ID, true, onlyProjectGuess
		return
	}

	// Title words count from line texts only, so a project called "Todo app"
	// doesn't gain a clue from every "todo:" line.
	bodyWords := map[string]bool{}
	for _, l := range b.lines {
		for w := range words(l.text) {
			bodyWords[w] = true
		}
	}
	score := map[int64]int{}
	for _, p := range projects {
		for w := range words(p.Title) {
			if bodyWords[w] {
				score[p.ID] += titleWordClue
			}
		}
	}
	for _, l := range b.lines {
		switch l.prefix {
		case prefixDone, prefixDoing:
			if t, _ := b.matchTask(l.text); t != nil {
				score[t.projectID] += matchClue
			}
		case prefixUnblocked:
			if bl, _ := b.matchBlocker(l.text); bl != nil {
				score[bl.projectID] += matchClue
			}
		}
	}

	best := projects[0].ID
	for _, p := range projects[1:] {
		if score[p.ID] > score[best] {
			best = p.ID
		}
	}
	b.project, b.projectGuessed = best, true
	b.projectConf = cluedProjectGuess
	if score[best] == 0 {
		b.projectConf = blindProjectGuess
	}
}

// line adds what one line produces. A prefixed line that yields nothing, such
// as "todo:" with no project to put the task in, is kept as a note.
func (b *builder) line(l line) error {
	var (
		made bool
		err  error
	)
	switch l.prefix {
	case prefixDone:
		made, err = b.setStatus(l, domain.TaskDone)
	case prefixDoing:
		made, err = b.setStatus(l, domain.TaskInProgress)
	case prefixTodo:
		if b.project != 0 {
			_, err = b.createTask(l)
			made = true
		}
	case prefixBlocked:
		b.observe(l, domain.ObservationBlocker)
		made = true
		if b.project != 0 {
			err = b.op(l, domain.OpCreateBlocker, b.projectConf, changeset.CreateBlocker{
				ProjectID: changeset.ID(b.project), Description: l.text,
			})
		}
	case prefixUnblocked:
		if bl, score := b.matchBlocker(l.text); bl != nil {
			bl.resolved = true
			made = true
			err = b.op(l, domain.OpResolveBlocker, score, changeset.ResolveBlocker{BlockerID: changeset.ID(bl.id)})
		}
	case prefixIdea:
		b.observe(l, domain.ObservationIdea)
		made = true
		p := changeset.ParkIdea{Title: l.text}
		conf := sureConfidence
		if b.project != 0 {
			ref := changeset.ID(b.project)
			p.ProjectID, conf = &ref, b.projectConf
		}
		err = b.op(l, domain.OpParkIdea, conf, p)
	case prefixFeel:
		b.observe(l, domain.ObservationFeeling)
		made = true
	case prefixDecided:
		b.observe(l, domain.ObservationDecision)
		made = true
	}
	if err != nil {
		return err
	}
	if !made {
		l.text = b.in.Entry.Body[l.start:l.end]
		b.observe(l, domain.ObservationNote)
	}
	return nil
}

// setStatus handles done: and doing:. It moves the best-matching task to
// status, or creates the task and then moves it. It reports false if it could
// do neither, for want of a project.
func (b *builder) setStatus(l line, status domain.TaskStatus) (bool, error) {
	t, score := b.matchTask(l.text)
	if t == nil {
		if b.project == 0 {
			return false, nil
		}
		b.observe(l, domain.ObservationProgress)
		var err error
		if t, err = b.createTask(l); err != nil {
			return false, err
		}
		score = b.projectConf
	} else {
		if !t.ref.IsTemp() {
			b.touched[t.ref.ID] = true
		}
		b.observe(l, domain.ObservationProgress, t)
	}
	if t.status == status {
		return true, nil // already there; nothing to propose
	}
	t.status = status
	return true, b.op(l, domain.OpSetTaskStatus, score, changeset.SetTaskStatus{TaskID: t.ref, Status: status})
}

// createTask proposes a new task in the entry's project and makes it a
// candidate for later lines.
func (b *builder) createTask(l line) (*taskCandidate, error) {
	err := b.op(l, domain.OpCreateTask, b.projectConf, changeset.CreateTask{
		ProjectID: changeset.ID(b.project), Title: l.text,
	})
	if err != nil {
		return nil, err
	}
	t := &taskCandidate{
		ref:       changeset.Temp(int64(len(b.res.Ops))),
		projectID: b.project,
		title:     l.text,
		status:    domain.TaskTodo,
	}
	b.tasks = append(b.tasks, t)
	return t, nil
}

// observe adds an observation over the line. If it names an existing task,
// it links to that task and the task's project. Otherwise it links to the
// entry's project, unless that was guessed without any clue: observations
// are stored as they are, and the review UI's project picker can't move them.
// Tasks created by this entry can't be linked: observations are stored
// before ops are applied.
func (b *builder) observe(l line, kind domain.ObservationKind, task ...*taskCandidate) {
	o := extract.ObservationDraft{Kind: kind, Summary: l.text, SpanStart: l.start, SpanEnd: l.end}
	switch {
	case len(task) > 0 && !task[0].ref.IsTemp():
		t := task[0]
		o.Links = append(o.Links,
			link(domain.TargetProject, t.projectID, sureConfidence),
			link(domain.TargetTask, t.ref.ID, sureConfidence))
	case b.project != 0 && b.projectConf != blindProjectGuess:
		o.Links = append(o.Links, link(domain.TargetProject, b.project, b.projectConf))
	}
	b.res.Observations = append(b.res.Observations, o)
}

func link(tt domain.TargetType, id int64, conf float64) extract.LinkDraft {
	return extract.LinkDraft{TargetType: tt, TargetID: id, Confidence: &conf}
}

// op appends an op for the line.
func (b *builder) op(l line, t domain.OpType, conf float64, p changeset.Payload) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("mock: marshal %s payload: %w", t, err)
	}
	rationale := fmt.Sprintf("from a %q line", string(l.prefix)+":")
	if b.projectGuessed && usesProject(p) {
		rationale += "; project guessed from the entry"
	}
	b.res.Ops = append(b.res.Ops, extract.ChangeOpDraft{OpType: t, Payload: raw, Rationale: rationale, Confidence: &conf})
	return nil
}

// usesProject reports whether the op puts something in the entry's project.
func usesProject(p changeset.Payload) bool {
	switch p := p.(type) {
	case changeset.CreateTask, changeset.CreateBlocker:
		return true
	case changeset.ParkIdea:
		return p.ProjectID != nil
	}
	return false
}

// matchTask returns the open task whose title best matches text, and the
// match score, or nil if none is close enough.
func (b *builder) matchTask(text string) (*taskCandidate, float64) {
	var best *taskCandidate
	bestScore := 0.0
	for _, t := range b.tasks {
		if s := similarity(text, t.title); s >= matchThreshold && s > bestScore {
			best, bestScore = t, s
		}
	}
	return best, bestScore
}

// matchBlocker returns the open blocker whose description best matches text.
func (b *builder) matchBlocker(text string) (*blockerCandidate, float64) {
	var best *blockerCandidate
	bestScore := 0.0
	for _, bl := range b.blockers {
		if bl.resolved {
			continue
		}
		if s := similarity(text, bl.description); s >= matchThreshold && s > bestScore {
			best, bestScore = bl, s
		}
	}
	return best, bestScore
}

// questions adds the follow-up questions from SPEC §10.4, at most
// extract.MaxQuestions of them.
func (b *builder) questions() {
	hasNext := slices.ContainsFunc(b.lines, func(l line) bool {
		return l.prefix == prefixTodo || l.prefix == prefixDoing
	})
	if !hasNext {
		b.ask("What's the next step?", "The entry doesn't name a next step.")
	}
	hasBlocker := slices.ContainsFunc(b.lines, func(l line) bool { return l.prefix == prefixBlocked })
	if t, ok := b.stalledTask(); ok && !hasBlocker {
		b.ask(fmt.Sprintf("Is anything in the way of %q?", t.Title),
			"It has been in progress for more than 3 days with no blocker noted.")
	}
}

func (b *builder) ask(text, reason string) {
	if len(b.res.Questions) < extract.MaxQuestions {
		b.res.Questions = append(b.res.Questions, extract.QuestionDraft{Text: text, Reason: reason})
	}
}

// stalledTask finds the task in the entry's project that has been in progress
// longest, past stalledAfter, with no open blocker on it or on the whole
// project. Tasks this entry named are skipped: the user just wrote about them.
func (b *builder) stalledTask() (extract.SnapshotTask, bool) {
	snap := b.in.Snapshot
	if snap.TakenAt.IsZero() || b.project == 0 {
		return extract.SnapshotTask{}, false
	}
	var (
		best  extract.SnapshotTask
		found bool
	)
	for _, p := range snap.Projects {
		if p.ID != b.project {
			continue
		}
		blocked := map[int64]bool{}
		for _, bl := range p.Blockers {
			if bl.TaskID == nil {
				return extract.SnapshotTask{}, false // the whole project is blocked
			}
			blocked[*bl.TaskID] = true
		}
		for _, t := range p.Tasks {
			if t.Status != domain.TaskInProgress || blocked[t.ID] || b.touched[t.ID] ||
				snap.TakenAt.Sub(t.UpdatedAt) <= stalledAfter {
				continue
			}
			if !found || t.UpdatedAt.Before(best.UpdatedAt) ||
				(t.UpdatedAt.Equal(best.UpdatedAt) && t.ID < best.ID) {
				best, found = t, true
			}
		}
	}
	return best, found
}

// stopWord reports whether w is left out of fuzzy matching and project clues.
func stopWord(w string) bool {
	switch w {
	case "a", "an", "and", "the", "to", "of", "in", "on", "for", "with", "is", "it",
		"my", "i", "at", "by", "or", "be", "this", "that":
		return true
	}
	return false
}

// words returns the lowercase words of s, without stop words.
func words(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if !stopWord(w) {
			out[w] = true
		}
	}
	return out
}

// similarity scores how well a and b name the same thing, from 0 to 1: the
// overlap of their words (Jaccard), raised to containmentScore when one's
// words all appear in the other.
func similarity(a, b string) float64 {
	wa, wb := words(a), words(b)
	if len(wa) == 0 || len(wb) == 0 {
		return 0
	}
	common := 0
	for w := range wa {
		if wb[w] {
			common++
		}
	}
	score := float64(common) / float64(len(wa)+len(wb)-common)
	if common == min(len(wa), len(wb)) {
		score = max(score, containmentScore)
	}
	return score
}
