package eval

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
)

// Totals sums outcomes over a whole eval run.
type Totals struct {
	Fixtures  int
	Ops       Counts
	Links     Counts
	Forbidden int
	Statuses  map[Status]int
	Latency   time.Duration
}

// Total adds up outcomes; precision and recall over the totals are
// micro-averaged, so a fixture with many ops weighs more.
func Total(outs []Outcome) Totals {
	t := Totals{Fixtures: len(outs), Statuses: map[Status]int{}}
	for _, o := range outs {
		t.Ops = t.Ops.Add(o.Score.Ops)
		t.Links = t.Links.Add(o.Score.Links)
		t.Forbidden += len(o.Score.Forbidden)
		t.Statuses[o.Status]++
		t.Latency += o.Latency
	}
	return t
}

// WriteReport prints one row per outcome and the totals. With verbose, each
// row is followed by the ops and links that were missing or extra, and the
// validator's problems or the extractor's error.
//
// Ops and links read "matched/expected +extra".
func WriteReport(w io.Writer, header string, outs []Outcome, verbose bool) error {
	b := &strings.Builder{}
	fmt.Fprintln(b, header)
	fmt.Fprintln(b)
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "fixture\trun\tops\tlinks\tforbidden\tquestions\ttime\t")
	for _, o := range outs {
		s := o.Score
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d/%d\t%s\t\n", o.Fixture, o.Status,
			counts(s.Ops), counts(s.Links), len(s.Forbidden), s.Questions, s.ExpectedQuestions, o.Latency.Round(100*time.Millisecond))
		if verbose {
			tw.Flush()
			details(b, o)
		}
	}
	tw.Flush()

	t := Total(outs)
	fmt.Fprintln(b)
	fmt.Fprintf(b, "ops    %s\n", summary(t.Ops))
	fmt.Fprintf(b, "links  %s\n", summary(t.Links))
	fmt.Fprintf(b, "forbidden ops proposed: %d\n", t.Forbidden)
	var runs []string
	for _, st := range Statuses() {
		runs = append(runs, fmt.Sprintf("%d %s", t.Statuses[st], st))
	}
	fmt.Fprintf(b, "runs: %s; %d fixtures in %s\n", strings.Join(runs, ", "), t.Fixtures, t.Latency.Round(time.Second))
	_, err := io.WriteString(w, b.String())
	return err
}

func counts(c Counts) string {
	s := fmt.Sprintf("%d/%d", c.Matched, c.Matched+c.Missing)
	if c.Extra > 0 {
		s += fmt.Sprintf(" +%d", c.Extra)
	}
	return s
}

func summary(c Counts) string {
	return fmt.Sprintf("P %.2f  R %.2f  F1 %.2f  (%d matched, %d missing, %d extra)",
		c.Precision(), c.Recall(), c.F1(), c.Matched, c.Missing, c.Extra)
}

func details(w io.Writer, o Outcome) {
	if o.Err != nil {
		fmt.Fprintf(w, "    error: %v\n", o.Err)
	}
	for _, p := range o.Problems {
		fmt.Fprintf(w, "    problem: %s\n", p)
	}
	for _, k := range o.Score.MissingOps {
		fmt.Fprintf(w, "    - op %s\n", k)
	}
	for _, k := range o.Score.ExtraOps {
		fmt.Fprintf(w, "    + op %s\n", k)
	}
	for _, k := range o.Score.MissingLinks {
		fmt.Fprintf(w, "    - link %s\n", k)
	}
	for _, k := range o.Score.ExtraLinks {
		fmt.Fprintf(w, "    + link %s\n", k)
	}
	for _, t := range o.Score.Forbidden {
		fmt.Fprintf(w, "    ! forbidden %s\n", t)
	}
}
