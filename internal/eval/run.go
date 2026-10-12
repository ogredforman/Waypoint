package eval

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// Status is how a fixture's run ended, in the pipeline's terms.
type Status string

const (
	StatusOK      Status = "ok"      // valid on the first try
	StatusRetried Status = "retried" // valid after the one retry
	StatusInvalid Status = "invalid" // rejected by the validator twice; the pipeline would fail the run
	StatusError   Status = "error"   // the extractor failed (timeout, unreachable, unusable reply)
)

// Statuses lists every Status in report order.
func Statuses() []Status { return []Status{StatusOK, StatusRetried, StatusInvalid, StatusError} }

// Outcome is one fixture's run and score.
type Outcome struct {
	Fixture  string
	Status   Status
	Problems []string // the validator's problems with the last rejected result
	Err      error    // the extractor's error, for StatusError
	Latency  time.Duration
	Score    Score
}

// Run runs each fixture through x as the pipeline would: extract, validate,
// and one retry told what was wrong if the result is rejected. A run that
// ends invalid or in error is scored as proposing nothing, which is what the
// user would get. done, if not nil, is called after each fixture. Run stops
// early if ctx is cancelled and returns the outcomes so far.
func Run(ctx context.Context, x extract.Extractor, fixtures []Fixture, done func(Outcome)) []Outcome {
	var out []Outcome
	for _, f := range fixtures {
		if ctx.Err() != nil {
			break
		}
		o := runOne(ctx, x, f)
		out = append(out, o)
		if done != nil {
			done(o)
		}
	}
	return out
}

func runOne(ctx context.Context, x extract.Extractor, f Fixture) Outcome {
	o := Outcome{Fixture: f.Name}
	in := f.Input
	start := time.Now()
	res, err := call(ctx, x, in)
	if err == nil {
		if verr := extract.Validate(in, res); verr != nil {
			o.Problems = problems(verr)
			prev, _ := json.Marshal(res)
			in.Retry = &extract.Retry{Previous: prev, Problems: o.Problems}
			res, err = call(ctx, x, in)
			o.Status = StatusRetried
			if err == nil {
				if verr := extract.Validate(in, res); verr != nil {
					o.Problems = problems(verr)
					o.Status = StatusInvalid
				}
			}
		} else {
			o.Status = StatusOK
		}
	}
	o.Latency = time.Since(start)
	if err != nil {
		o.Status, o.Err = StatusError, err
	}
	if o.Status != StatusOK && o.Status != StatusRetried {
		res = extract.ExtractionResult{}
	}
	o.Score = ScoreResult(f, res)
	return o
}

// call runs the extractor, turning a panic into an error.
func call(ctx context.Context, x extract.Extractor, in extract.ExtractInput) (res extract.ExtractionResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("extractor panicked")
		}
	}()
	return x.Extract(ctx, in)
}

// problems lists a validation error's problems as "field: problem" lines.
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
