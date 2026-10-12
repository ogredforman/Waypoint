package eval

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/ogredforman/waypoint/internal/changeset"
	"github.com/ogredforman/waypoint/internal/domain"
	"github.com/ogredforman/waypoint/internal/extract"
)

// Counts compares what was expected with what came back: Matched are in
// both, Missing only in the expected result, Extra only in the actual one.
type Counts struct {
	Matched, Missing, Extra int
}

// Add sums two counts.
func (c Counts) Add(o Counts) Counts {
	return Counts{Matched: c.Matched + o.Matched, Missing: c.Missing + o.Missing, Extra: c.Extra + o.Extra}
}

// Precision is the share of what came back that was expected; 1 if nothing
// came back.
func (c Counts) Precision() float64 { return ratio(c.Matched, c.Matched+c.Extra) }

// Recall is the share of what was expected that came back; 1 if nothing was
// expected.
func (c Counts) Recall() float64 { return ratio(c.Matched, c.Matched+c.Missing) }

// F1 is the harmonic mean of precision and recall.
func (c Counts) F1() float64 {
	p, r := c.Precision(), c.Recall()
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

func ratio(n, d int) float64 {
	if d == 0 {
		return 1
	}
	return float64(n) / float64(d)
}

// Score is how one result compares with a fixture's expected result. Free
// text (summaries, titles, rationales) isn't scored; it has many good answers.
type Score struct {
	// Ops match on op type, the ref fields of the payload (a temp ref counts
	// as "new", whatever its position), and status. An op proposing the right
	// thing with a different optional ref, such as another milestone, doesn't
	// match.
	Ops                          Counts
	MissingOps, ExtraOps         []string // op keys, for the verbose report
	Links                        Counts   // the set of link targets over all observations
	MissingLinks, ExtraLinks     []string
	Forbidden                    []domain.OpType // proposed op types the fixture forbids
	Questions, ExpectedQuestions int
}

// ScoreResult compares got with f's expected result. A run that produced
// nothing (it failed) is scored with an empty result.
func ScoreResult(f Fixture, got extract.ExtractionResult) Score {
	s := Score{Questions: len(got.Questions), ExpectedQuestions: len(f.Expected.Questions)}
	s.Ops, s.MissingOps, s.ExtraOps = compare(opKeys(f.Expected.Ops), opKeys(got.Ops))
	s.Links, s.MissingLinks, s.ExtraLinks = compare(linkKeys(f.Expected.Observations), linkKeys(got.Observations))
	for _, op := range got.Ops {
		if slices.Contains(f.Forbid, op.OpType) {
			s.Forbidden = append(s.Forbidden, op.OpType)
		}
	}
	return s
}

// compare matches the keys in want and got one to one.
func compare(want, got []string) (c Counts, missing, extra []string) {
	left := map[string]int{}
	for _, k := range got {
		left[k]++
	}
	for _, k := range want {
		if left[k] > 0 {
			left[k]--
			c.Matched++
		} else {
			missing = append(missing, k)
		}
	}
	for _, k := range got {
		if left[k] > 0 {
			left[k]--
			extra = append(extra, k)
		}
	}
	c.Missing, c.Extra = len(missing), len(extra)
	return c, missing, extra
}

func opKeys(ops []extract.ChangeOpDraft) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = opKey(op)
	}
	return out
}

// opKey names an op by what it does to which rows, e.g.
// "set_task_status{status=done task_id=10}".
func opKey(op extract.ChangeOpDraft) string {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(op.Payload, &fields); err != nil {
		return string(op.OpType) + "{?}"
	}
	var parts []string
	for name, raw := range fields {
		switch {
		case strings.HasSuffix(name, "_id"):
			parts = append(parts, name+"="+refValue(raw))
		case name == "status":
			var v string
			if json.Unmarshal(raw, &v) != nil {
				v = "?"
			}
			parts = append(parts, name+"="+v)
		}
	}
	sort.Strings(parts)
	return string(op.OpType) + "{" + strings.Join(parts, " ") + "}"
}

func refValue(raw json.RawMessage) string {
	if string(raw) == "null" {
		return "none"
	}
	var r changeset.Ref
	if json.Unmarshal(raw, &r) != nil || r.Problem() != "" {
		return "?"
	}
	if r.IsTemp() {
		return "new"
	}
	return fmt.Sprint(r.ID)
}

// linkKeys is the set of rows the observations link to, e.g. "task 12" or
// "task new". Which observation carries a link isn't scored: models split an
// entry into observations differently.
func linkKeys(obs []extract.ObservationDraft) []string {
	seen := map[string]bool{}
	var out []string
	for _, o := range obs {
		for _, l := range o.Links {
			k := string(l.TargetType) + " "
			if l.TargetID.IsTemp() {
				k += "new"
			} else {
				k += fmt.Sprint(l.TargetID.ID)
			}
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}
