package extract

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
)

// Validate checks an extractor's output against the entry and snapshot it was
// given. Every extractor's output goes through it before anything is stored.
//
// It checks enums, required text, and confidences in [0, 1]; that spans fall
// within the entry body on character boundaries; that ops are well formed and
// their temp refs point at earlier creating ops (changeset.ValidateOps); that
// a link's temp ref names an op that creates that kind of entity; that every
// ref to an existing row, in ops and links, names a row in the snapshot;
// and that there are at most MaxQuestions questions. Cross-row rules, such as
// a blocker's task being in its project, are left to the applier.
//
// All problems come back in one *domain.ValidationError, so they can be fed
// back to a model. Fields are named "observations[i]" and "questions[i]" by
// index from 0, and "ops[N]" by position from 1, the N of "$new:N". Problems
// never echo text from the entry or the output.
func Validate(in ExtractInput, res ExtractionResult) error {
	var c, opc checker
	body := in.Entry.Body

	// Ops are checked first, since temp links need to know what they create,
	// but their problems are reported after the observations'.
	payloads, err := opc.ops(in.Snapshot, res.Ops)
	if err != nil {
		return err
	}
	creates := map[int64]domain.TargetType{} // position -> kind created
	for i, p := range payloads {
		if p == nil {
			continue
		}
		if kind, ok := p.Creates(); ok {
			creates[int64(i+1)] = kind
		}
	}

	for i, o := range res.Observations {
		prefix := fmt.Sprintf("observations[%d].", i)
		c.enum(prefix+"kind", o.Kind.Valid())
		c.notBlank(prefix+"summary", o.Summary)
		c.span(prefix, body, o.SpanStart, o.SpanEnd)
		type target struct {
			kind domain.TargetType
			ref  changeset.Ref
		}
		seen := map[target]bool{} // observation_links is unique per target
		for j, l := range o.Links {
			lp := fmt.Sprintf("%slinks[%d].", prefix, j)
			c.link(lp, in.Snapshot, creates, l)
			t := target{l.TargetType, l.TargetID}
			if seen[t] {
				c.add(lp+"target_id", "duplicates an earlier link")
			}
			seen[t] = true
		}
	}

	c.problems = append(c.problems, opc.problems...)

	if n := len(res.Questions); n > MaxQuestions {
		c.add("questions", "at most %d are allowed, got %d", MaxQuestions, n)
	}
	for i, q := range res.Questions {
		prefix := fmt.Sprintf("questions[%d].", i)
		c.notBlank(prefix+"text", q.Text)
		c.notBlank(prefix+"reason", q.Reason)
	}

	if len(c.problems) == 0 {
		return nil
	}
	return &domain.ValidationError{Entity: "extraction result", Problems: c.problems}
}

// checker collects problems while a result is validated.
type checker struct {
	problems []domain.FieldError
}

func (c *checker) add(field, format string, args ...any) {
	c.problems = append(c.problems, domain.FieldError{Field: field, Problem: fmt.Sprintf(format, args...)})
}

func (c *checker) notBlank(field, value string) {
	if strings.TrimSpace(value) == "" {
		c.add(field, "is required")
	}
}

// enum doesn't echo the value: it comes from extractor output.
func (c *checker) enum(field string, valid bool) {
	if !valid {
		c.add(field, "is not an allowed value")
	}
}

func (c *checker) confidence(field string, v float64) {
	if math.IsNaN(v) || v < 0 || v > 1 {
		c.add(field, "must be between 0 and 1")
	}
}

// span checks that [start, end) is a byte range within body whose ends fall on
// UTF-8 character boundaries.
func (c *checker) span(prefix, body string, start, end int64) {
	n := int64(len(body))
	switch {
	case start < 0 || start > n:
		c.add(prefix+"span_start", "must be within the entry (0 to %d bytes)", n)
	case !boundary(body, start):
		c.add(prefix+"span_start", "must not split a character")
	}
	switch {
	case end < start || end > n:
		c.add(prefix+"span_end", "must be within the entry, from span_start to %d bytes", n)
	case !boundary(body, end):
		c.add(prefix+"span_end", "must not split a character")
	}
}

// boundary reports whether byte offset i in s starts a character or is the
// end of s. i must be within [0, len(s)].
func boundary(s string, i int64) bool {
	return i == int64(len(s)) || utf8.RuneStart(s[i])
}

// link checks one link. creates maps op positions to the kind of entity each
// creating op makes, for temp refs.
func (c *checker) link(prefix string, snap StateSnapshot, creates map[int64]domain.TargetType, l LinkDraft) {
	if l.Confidence == nil {
		c.add(prefix+"confidence", "is required")
	} else {
		c.confidence(prefix+"confidence", *l.Confidence)
	}
	switch {
	case !l.TargetType.Valid():
		c.enum(prefix+"target_type", false)
	case l.TargetType == domain.TargetMilestone:
		c.add(prefix+"target_type", "milestones aren't supported yet")
	case l.TargetID.Problem() != "":
		c.add(prefix+"target_id", "%s", l.TargetID.Problem())
	case l.TargetID.IsTemp():
		pos := l.TargetID.Temp
		kind, ok := creates[pos]
		switch {
		case !ok:
			c.add(prefix+"target_id", "refers to position %d, where no op creates anything", pos)
		case kind != l.TargetType:
			c.add(prefix+"target_id", "refers to position %d, which creates a %s, not a %s", pos, kind, l.TargetType)
		}
	case !snap.Has(l.TargetType, l.TargetID.ID):
		c.add(prefix+"target_id", "%s %d is not in the snapshot", l.TargetType, l.TargetID.ID)
	}
}

// ops validates the ops as one changeset, then checks their refs to existing
// rows against the snapshot. It returns the decoded payloads, nil where one
// didn't decode. An error that isn't a validation problem is returned as is.
func (c *checker) ops(snap StateSnapshot, drafts []ChangeOpDraft) ([]changeset.Payload, error) {
	ops := make([]domain.ChangeOp, len(drafts))
	for i, d := range drafts {
		ops[i] = domain.ChangeOp{Position: int64(i + 1), OpType: d.OpType, Payload: d.Payload}
		if d.Confidence != nil {
			c.confidence(fmt.Sprintf("ops[%d].confidence", i+1), *d.Confidence)
		}
	}

	payloads, err := changeset.ValidateOps(ops)
	var ve *domain.ValidationError
	if errors.As(err, &ve) {
		c.problems = append(c.problems, ve.Problems...)
	} else if err != nil {
		return nil, err
	}

	for i, p := range payloads {
		if p == nil {
			continue // didn't decode; already reported
		}
		for _, r := range changeset.Refs(p) {
			// Temp refs were checked by ValidateOps, as were malformed refs.
			if r.Ref.IsTemp() || r.Ref.ID <= 0 {
				continue
			}
			if !snap.Has(r.Kind, r.Ref.ID) {
				c.add(fmt.Sprintf("ops[%d].%s", i+1, r.Field), "%s %d is not in the snapshot", r.Kind, r.Ref.ID)
			}
		}
	}
	return payloads, nil
}
