package extract

import (
	"testing"

	"github.com/ogredforman/waypoint/internal/domain"
)

func TestResolveQuotes(t *testing.T) {
	type span struct{ start, end int64 }
	tests := []struct {
		name   string
		body   string
		quotes []string
		want   []span
	}{
		{"exact", "Fixed the login bug today.", []string{"login bug"}, []span{{10, 19}}},
		{"empty quote", "Fixed it.", []string{""}, []span{{0, 0}}},
		{"blank quote", "Fixed it.", []string{"  "}, []span{{0, 0}}},
		{"not found", "Fixed it.", []string{"broke it"}, []span{{0, 0}}},
		{
			"repeat takes the one after the previous quote",
			"tests pass. then tests pass again.",
			[]string{"then", "tests pass"},
			[]span{{12, 16}, {17, 27}},
		},
		{
			"out of order falls back to the start",
			"first thing, second thing",
			[]string{"second", "first"},
			[]span{{13, 19}, {0, 5}},
		},
		{
			"a miss doesn't move the cursor",
			"a b a b",
			[]string{"b", "nope", "a"},
			[]span{{2, 3}, {0, 0}, {4, 5}},
		},
		{
			"whitespace differs",
			"wrote the\n  parser   today",
			[]string{"the parser today"},
			[]span{{6, 26}},
		},
		{
			"quote marks differ",
			"she said “ship it” and left",
			[]string{`said "ship it"`},
			[]span{{4, 22}},
		},
		{
			"curly apostrophe in the quote",
			"it's done",
			[]string{"it’s done"},
			[]span{{0, 9}},
		},
		{
			"quote has surrounding whitespace",
			"one  two three",
			[]string{" two three\n"},
			[]span{{5, 14}},
		},
		{
			"multibyte characters",
			body, // "café ☕ done: ship it"
			[]string{"☕ done", "café"},
			[]span{{6, 14}, {0, 5}},
		},
		{
			"normalised match falls back to the start",
			"go  home. go\thome now",
			[]string{"now", "go home"},
			[]span{{18, 21}, {0, 8}},
		},
		{
			"normalised repeat after the cursor",
			"go  home. then go\thome",
			[]string{"then", "go home"},
			[]span{{10, 14}, {15, 22}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obs := make([]ObservationDraft, len(tt.quotes))
			for i, q := range tt.quotes {
				// A stale span must be overwritten.
				obs[i] = ObservationDraft{Kind: domain.ObservationNote, Summary: "s", Quote: q, SpanStart: 1, SpanEnd: 2}
			}
			ResolveQuotes(tt.body, obs)
			for i, o := range obs {
				if got := (span{o.SpanStart, o.SpanEnd}); got != tt.want[i] {
					t.Errorf("quote %d: span = %v (%q), want %v", i, got, tt.body[o.SpanStart:o.SpanEnd], tt.want[i])
				}
			}
			in := ExtractInput{Entry: domain.Entry{Body: tt.body}}
			if err := Validate(in, ExtractionResult{Observations: obs}); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}
