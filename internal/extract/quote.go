package extract

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ResolveQuotes sets each observation's span from its quote, a phrase the
// model copied from the entry body. Observations are taken in order, and a
// quote is looked for first after the previous quote's match, then anywhere
// in the body (models often group observations by kind, not by position). An
// exact match is tried before one that ignores differences in whitespace and
// quote marks. An empty quote, or one that isn't found, gets an empty span at
// 0. The spans fall on character boundaries, so Validate accepts them.
func ResolveQuotes(body string, obs []ObservationDraft) {
	norm := normalize(body)
	cursor := 0 // byte offset in body after the previous match
	for i := range obs {
		obs[i].SpanStart, obs[i].SpanEnd = 0, 0
		start, end, ok := findQuote(body, norm, obs[i].Quote, cursor)
		if !ok {
			continue
		}
		obs[i].SpanStart, obs[i].SpanEnd = int64(start), int64(end)
		cursor = end
	}
}

// findQuote returns the byte range of q in body, preferring a match at or
// after from.
func findQuote(body string, norm normalized, q string, from int) (start, end int, ok bool) {
	if strings.TrimSpace(q) == "" {
		return 0, 0, false
	}
	if i := strings.Index(body[from:], q); i >= 0 {
		return from + i, from + i + len(q), true
	}
	if i := strings.Index(body, q); i >= 0 {
		return i, i + len(q), true
	}

	nq := normalize(strings.TrimSpace(q)).text
	nfrom := norm.at(from)
	if i := strings.Index(norm.text[nfrom:], nq); i >= 0 {
		return norm.span(nfrom+i, len(nq))
	}
	if i := strings.Index(norm.text, nq); i >= 0 {
		return norm.span(i, len(nq))
	}
	return 0, 0, false
}

// normalized is a string with every whitespace run collapsed to one space and
// curly quotes made straight. starts[i] and ends[i] are the byte range in the
// original string that the normalized text's byte i came from.
type normalized struct {
	text         string
	starts, ends []int
}

func normalize(s string) normalized {
	var n normalized
	var b strings.Builder
	emit := func(r rune, start, end int) {
		before := b.Len()
		b.WriteRune(r)
		for range b.Len() - before {
			n.starts = append(n.starts, start)
			n.ends = append(n.ends, end)
		}
	}
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			j := i + size
			for j < len(s) {
				r2, size2 := utf8.DecodeRuneInString(s[j:])
				if !unicode.IsSpace(r2) {
					break
				}
				j += size2
			}
			emit(' ', i, j)
			i = j
			continue
		}
		switch r {
		case '‘', '’', '‚', '‛', '′':
			r = '\''
		case '“', '”', '„', '‟', '″':
			r = '"'
		}
		emit(r, i, i+size)
		i += size
	}
	n.text = b.String()
	return n
}

// at returns the first normalized byte that comes from original offset off or
// later.
func (n normalized) at(off int) int {
	for i, s := range n.starts {
		if s >= off {
			return i
		}
	}
	return len(n.text)
}

// span maps a match of length l at normalized byte i back to the original.
func (n normalized) span(i, l int) (start, end int, ok bool) {
	return n.starts[i], n.ends[i+l-1], true
}
