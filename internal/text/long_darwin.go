package text

import "sort"

// A long text is typeset a window at a time (typesetWindows), and each line
// only from where it starts: so the next text of the same style, as a
// keystroke in a long paragraph makes it, takes the lines before the change
// from the last and, once a line starts where one did in the unchanged text
// after it, the lines from there, moved by what the change added. Only the
// lines between are typeset again.

// longKey is all that typesets a long text but its runes and spans.
type longKey struct {
	font                    uintptr
	rtl, italic, noKern     bool
	lang, features          string
	spacing, tabStop, width float32
	wholeWords, keepWords   bool
}

// longText is a long text as it was last typeset: its runes, its spans and
// its lines, whose runs and glyphs are its own.
type longText struct {
	key   longKey
	text  []rune
	spans []Span
	lines []shapedLine
	used  uint64
}

const (
	// longKept is how many long texts stay typeset.
	longKept = 4
	// longReach is how far from a change a line may end, or start, and
	// still be taken as it was: what breaks a line may look past its end,
	// and segmenting words before its start.
	longReach = 64
)

// lastLong is the long text last typeset under key, if any.
func (e *coreText) lastLong(key longKey) *longText {
	for _, l := range e.long {
		if l.key == key {
			l.used = e.frame
			return l
		}
	}
	return nil
}

// keepLong keeps a long text as typeset, in its key's place or the least
// recently used one's.
func (e *coreText) keepLong(l *longText) {
	l.used = e.frame
	for i, k := range e.long {
		if k.key == l.key {
			e.long[i] = l
			return
		}
	}
	if len(e.long) < longKept {
		e.long = append(e.long, l)
		return
	}
	old := 0
	for i, k := range e.long {
		if k.used < e.long[old].used {
			old = i
		}
	}
	e.long[old] = l
}

// changed is how many runes old and text share at their start and at their
// end, with the same spans: the end's no more than what the start leaves.
func changed(old, text []rune, oldSpans, spans []Span) (head, tail int) {
	n := min(len(old), len(text))
	for head < n && old[head] == text[head] {
		head++
	}
	head = min(head, sameSpansFrom(oldSpans, spans, len(old), len(text), false))
	for tail < n-head && old[len(old)-1-tail] == text[len(text)-1-tail] {
		tail++
	}
	tail = min(tail, sameSpansFrom(oldSpans, spans, len(old), len(text), true), n-head)
	return head, tail
}

// sameSpansFrom is how many runes from the start of two texts, or from their
// end, the spans of a and of b style alike.
func sameSpansFrom(a, b []Span, na, nb int, fromEnd bool) int {
	at := func(spans []Span, n, d int) (Span, int) {
		// The span d runes from the start (or end), and how many more runes
		// on it goes.
		pos := d
		if fromEnd {
			pos = n - 1 - d
		}
		i := sort.Search(len(spans), func(i int) bool { return spans[i].End > pos })
		if i == len(spans) {
			// Past the last span, the text's own style.
			if fromEnd {
				last := 0
				if len(spans) > 0 {
					last = spans[len(spans)-1].End
				}
				return Span{}, pos - last + 1
			}
			return Span{}, n - pos
		}
		sp := spans[i]
		left := sp.End - pos
		if fromEnd {
			start := 0
			if i > 0 {
				start = spans[i-1].End
			}
			left = pos - start + 1
		}
		sp.End = 0
		return sp, left
	}
	n := min(na, nb)
	for d := 0; d < n; {
		sa, la := at(a, na, d)
		sb, lb := at(b, nb, d)
		if sa != sb {
			return d
		}
		d += max(min(la, lb), 1)
	}
	return n
}

// reuse is what of the lines last typeset a text, changed from them, takes:
// the lines before the change, the rune where typesetting starts again,
// and rest, which tells, for a line typeset again ending at a rune, the
// lines last typeset from there on, to be moved by what the change added,
// or nil.
func (l *longText) reuse(text []rune, spans []Span) (head []shapedLine, from int, rest func(at int) []shapedLine) {
	h, t := changed(l.text, text, l.spans, spans)
	k := 0
	for k < len(l.lines) && l.lines[k].end+longReach <= h {
		k++
	}
	from = 0
	if k > 0 {
		from = l.lines[k-1].end
	}
	delta := len(text) - len(l.text)
	unchanged := len(text) - t
	rest = func(at int) []shapedLine {
		// A line starting at at, well within the unchanged end, starts
		// where one did.
		if at-longReach < unchanged || at >= len(text) {
			return nil
		}
		was := at - delta
		i := sort.Search(len(l.lines), func(i int) bool { return l.lines[i].start >= was })
		if i == len(l.lines) || l.lines[i].start != was {
			return nil
		}
		return l.lines[i:]
	}
	return l.lines[:k], from, rest
}

// ownLines copies lines into room of their own, each rune moved by delta.
func ownLines(lines []shapedLine, delta int) []shapedLine {
	glyphs, runs := 0, 0
	for _, sl := range lines {
		runs += len(sl.runs)
		for _, r := range sl.runs {
			glyphs += len(r.glyphs)
		}
	}
	gs := make([]Glyph, 0, glyphs)
	rs := make([]shapedRun, 0, runs)
	out := make([]shapedLine, len(lines))
	for i, sl := range lines {
		r0 := len(rs)
		for _, r := range sl.runs {
			g0 := len(gs)
			gs = append(gs, r.glyphs...)
			for j := g0; j < len(gs); j++ {
				if gs[j].Cluster >= 0 {
					gs[j].Cluster += delta
				}
			}
			r.glyphs = gs[g0:len(gs):len(gs)]
			r.start += delta
			r.end += delta
			rs = append(rs, r)
		}
		sl.runs = rs[r0:len(rs):len(rs)]
		sl.start += delta
		sl.end += delta
		out[i] = sl
	}
	return out
}

// scratchLines copies lines into the scratch, as shape returns them: whoever
// takes them may change them.
func (e *coreText) scratchLines(mark int, lines []shapedLine) []shapedLine {
	var out []shapedLine
	for _, sl := range lines {
		r0 := len(e.shapeScratch.runs)
		var runs []shapedRun
		for _, r := range sl.runs {
			g := e.glyphRoom(len(r.glyphs))
			copy(g, r.glyphs)
			r.glyphs = g
			runs = e.addRun(r0, r)
		}
		sl.runs = runs
		out = e.addLine(mark, sl)
	}
	return out
}
