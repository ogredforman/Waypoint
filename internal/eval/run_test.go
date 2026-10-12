package eval

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// scripted returns its replies in order, one per call, and records the
// inputs it was given.
type scripted struct {
	replies []func() (extract.ExtractionResult, error)
	inputs  []extract.ExtractInput
}

func (s *scripted) Name() string { return "scripted" }

func (s *scripted) Extract(_ context.Context, in extract.ExtractInput) (extract.ExtractionResult, error) {
	s.inputs = append(s.inputs, in)
	r := s.replies[0]
	s.replies = s.replies[1:]
	return r()
}

func reply(res extract.ExtractionResult, err error) func() (extract.ExtractionResult, error) {
	return func() (extract.ExtractionResult, error) { return res, err }
}

func TestRun(t *testing.T) {
	fs, err := LoadFixtures("../../testdata/entries")
	if err != nil {
		t.Fatal(err)
	}
	f := fs[0]
	// An op naming a task that isn't in the snapshot fails validation.
	invalid := extract.ExtractionResult{Ops: []extract.ChangeOpDraft{op(domain.OpSetTaskStatus, `{"task_id":99999,"status":"done"}`)}}

	for _, tc := range []struct {
		name    string
		replies []func() (extract.ExtractionResult, error)
		status  Status
		calls   int
		scored  bool // scored on the expected result rather than nothing
	}{
		{"ok", []func() (extract.ExtractionResult, error){reply(f.Expected, nil)}, StatusOK, 1, true},
		{"retried", []func() (extract.ExtractionResult, error){reply(invalid, nil), reply(f.Expected, nil)}, StatusRetried, 2, true},
		{"invalid", []func() (extract.ExtractionResult, error){reply(invalid, nil), reply(invalid, nil)}, StatusInvalid, 2, false},
		{"error", []func() (extract.ExtractionResult, error){reply(extract.ExtractionResult{}, errors.New("timeout"))}, StatusError, 1, false},
		{"panic", []func() (extract.ExtractionResult, error){func() (extract.ExtractionResult, error) { panic("boom") }}, StatusError, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := &scripted{replies: tc.replies}
			var seen []string
			outs := Run(context.Background(), x, []Fixture{f}, func(o Outcome) { seen = append(seen, o.Fixture) })
			if len(outs) != 1 || len(seen) != 1 {
				t.Fatalf("outcomes %d, done calls %d", len(outs), len(seen))
			}
			o := outs[0]
			if o.Status != tc.status || len(x.inputs) != tc.calls {
				t.Fatalf("status %s after %d calls, want %s after %d (err %v, problems %v)", o.Status, len(x.inputs), tc.status, tc.calls, o.Err, o.Problems)
			}
			if tc.calls == 2 && (x.inputs[1].Retry == nil || len(x.inputs[1].Retry.Problems) == 0) {
				t.Error("retry wasn't told what was wrong")
			}
			if tc.scored != (o.Score.Ops.Missing == 0 && o.Score.Links.Missing == 0) {
				t.Errorf("score %+v", o.Score)
			}
		})
	}

	t.Run("cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if outs := Run(ctx, &scripted{}, fs, nil); len(outs) != 0 {
			t.Errorf("ran %d fixtures after cancel", len(outs))
		}
	})
}

func TestWriteReport(t *testing.T) {
	outs := []Outcome{
		{Fixture: "01-a", Status: StatusOK, Score: Score{Ops: Counts{Matched: 1}, Links: Counts{Matched: 2, Missing: 1, Extra: 1}, MissingLinks: []string{"task 12"}}},
		{Fixture: "02-b", Status: StatusInvalid, Problems: []string{"ops[1]: task 9 isn't in the snapshot"}, Score: Score{Ops: Counts{Missing: 1}, Forbidden: []domain.OpType{domain.OpDeleteTask}}},
	}
	var b bytes.Buffer
	if err := WriteReport(&b, "waypoint eval: test", outs, true); err != nil {
		t.Fatal(err)
	}
	got := b.String()
	for _, want := range []string{
		"waypoint eval: test",
		"01-a", "2/3 +1", "    - link task 12",
		"02-b", "invalid", "    problem: ops[1]: task 9",
		"ops    P 1.00  R 0.50  F1 0.67  (1 matched, 1 missing, 0 extra)",
		"forbidden ops proposed: 1",
		"runs: 1 ok, 0 retried, 1 invalid, 0 error; 2 fixtures",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
}
