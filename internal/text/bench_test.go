package text

import (
	"strconv"
	"strings"
	"testing"
)

// BenchmarkLayout lays out a wrapped paragraph without the cache, as each
// keystroke in an editor does.
func BenchmarkLayout(b *testing.B) {
	s := Shared()
	p := Params{Text: "The quick brown fox jumps over the lazy dog, and then some more words follow to wrap the paragraph over a few lines.", Style: Style{Size: 14}, Width: 300}
	s.Layout(p)
	b.ResetTimer()
	for range b.N {
		// What a keystroke's new text finds: nothing of its shaping kept.
		s.ReleaseShaping()
		s.mu.Lock()
		s.layout(p)
		s.mu.Unlock()
	}
}

// BenchmarkLayoutLabel lays out a short label without the cache, as each
// text of a list's row coming into view does, and BenchmarkLayoutTruncated
// a line cut short with an ellipsis.
func BenchmarkLayoutLabel(b *testing.B) {
	benchLayout(b, Params{Text: "Grace Hopper", Style: Style{Size: 12}})
}

func BenchmarkLayoutTruncated(b *testing.B) {
	benchLayout(b, Params{Text: "fix the list scroll when rows change height and keep focus", Style: Style{Size: 12}, Width: 160, MaxLines: 1})
}

func BenchmarkLayoutTruncatedMiddle(b *testing.B) {
	Shared().UseTruncateMiddle()
	benchLayout(b, Params{Text: "src/components/settings/appearance/theme/picker.go", Style: Style{Size: 12}, Width: 160, MaxLines: 1, TruncateMiddle: true})
}

func BenchmarkLayoutTruncatedStart(b *testing.B) {
	Shared().UseTruncateStart()
	benchLayout(b, Params{Text: "src/components/settings/appearance/theme/picker.go", Style: Style{Size: 12}, Width: 160, MaxLines: 1, TruncateStart: true})
}

func benchLayout(b *testing.B, p Params) {
	s := Shared()
	s.Layout(p)
	b.ReportAllocs()
	for b.Loop() {
		s.mu.Lock()
		s.layout(p)
		s.mu.Unlock()
	}
}

// BenchmarkLayoutDocument lays out the paragraphs of a document taking a
// little more than the cache's budget, as each frame of a window showing
// it does, with the cache.
func BenchmarkLayoutDocument(b *testing.B) {
	s := newSystem()
	ps := document(s)
	for _, p := range ps {
		s.Layout(p)
	}
	s.EndFrame()
	made := s.LayoutsMade()
	b.ReportAllocs()
	for b.Loop() {
		for _, p := range ps {
			s.Layout(p)
		}
		s.EndFrame()
	}
	b.ReportMetric(float64(s.LayoutsMade()-made)/float64(b.N), "layouts/op")
}

// BenchmarkLayoutResize wraps the 40 paragraphs of a document to another width
// in each frame, as a window being resized does, or a sidebar sliding, and
// BenchmarkLayoutResizeChinese in a language Core Text breaks lines of with a
// break iterator of its own.
func BenchmarkLayoutResize(b *testing.B) {
	benchResize(b, "window button layout render scroll update shadow cursor memory thread signal bitmap stroke column editor kernel", "en")
}

func BenchmarkLayoutResizeChinese(b *testing.B) {
	benchResize(b, "窗口 按钮 布局 渲染 滚动 更新 阴影 光标 内存 线程 信号 位图 笔画 栏目 编辑器 内核", "zh-Hans")
}

func benchResize(b *testing.B, list, lang string) {
	s := newSystem()
	words := strings.Fields(list)
	var paragraphs []string
	for i := range 40 {
		var p strings.Builder
		for j := range 70 {
			p.WriteString(words[(i*7+j*j*3+j)%len(words)])
			p.WriteByte(' ')
		}
		paragraphs = append(paragraphs, p.String())
	}
	width := float32(450)
	b.ReportAllocs()
	for b.Loop() {
		if width++; width > 750 {
			width = 450
		}
		for _, p := range paragraphs {
			s.Layout(Params{Text: p, Width: width, Style: Style{Size: 13, Lang: lang}})
		}
		s.EndFrame()
	}
}

// BenchmarkLayoutNumbers lays out the 600 numbers of a dashboard, each new in
// each frame.
func BenchmarkLayoutNumbers(b *testing.B) {
	s := newSystem()
	frame := 0
	b.ReportAllocs()
	for b.Loop() {
		frame++
		for i := range 600 {
			v := (i*7919 + frame*31) % 100000
			s.Layout(Params{Text: strconv.Itoa(v/100) + "." + strconv.Itoa(v%100 + 100)[1:], NoWrap: true, Style: Style{Size: 11}})
		}
		s.EndFrame()
	}
}

func BenchmarkGlyphCache(b *testing.B) {
	s := newSystem()
	l := s.Layout(Params{
		Text:  "The quick brown fox jumps over the lazy dog 0123456789 窗口布局渲染滚动更新阴影光标内存信号",
		Style: Style{Size: 14},
		Width: 2000,
	})
	var glyphs []Glyph
	for _, ln := range l.Lines {
		glyphs = append(glyphs, ln.Glyphs...)
	}
	for _, g := range glyphs {
		s.Glyph(g.Font, g.ID, 2, g.X*2, 0, false)
	}

	b.Run("hit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			for _, g := range glyphs {
				s.Glyph(g.Font, g.ID, 2, g.X*2, 0, false)
			}
		}
	})

	b.Run("hit_locked", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			s.Lock()
			for _, g := range glyphs {
				s.GlyphLocked(g.Font, g.ID, 2, g.X*2, 0, false)
			}
			s.Unlock()
		}
	})

	b.Run("miss", func(b *testing.B) {
		eng := s.engine()
		b.ReportAllocs()
		for b.Loop() {
			for i, g := range glyphs {
				eng.glyph(g.Font, g.ID, 2, float32(i%4)/4, 0, false)
			}
		}
	})
}

func longBenchParagraph(minBytes int) string {
	const sentence = "The quick brown fox jumps over a lazy dog by the river window while writing clean prose for long documents. "
	var b strings.Builder
	b.Grow(minBytes + len(sentence))
	for b.Len() < minBytes {
		b.WriteString(sentence)
	}
	return b.String()
}

// BenchmarkLayoutLong lays out a wrapped paragraph past typesetLong (8,192
// runes): cold releases shaping on every iteration (measuring windowed linear
// typesetting), while edit alternates distinct one-word mutations in the
// middle across consecutive frames without dropping shaping (measuring
// incremental long-text line reuse).
func BenchmarkLayoutLong(b *testing.B) {
	sizes := []struct {
		name  string
		bytes int
	}{
		{"16KiB", 16 << 10},
		{"64KiB", 64 << 10},
		{"256KiB", 256 << 10},
	}
	for _, tc := range sizes {
		a := longBenchParagraph(tc.bytes)
		mid := len(a) / 2
		off := strings.Index(a[mid:], "fox")
		variants := make([]Params, 16)
		for k := range variants {
			word := "fog" + strconv.Itoa(10+k)
			if k%2 == 1 {
				word = "fogg" + strconv.Itoa(10+k)
			}
			variants[k] = Params{
				Text:  a[:mid+off] + word + a[mid+off+3:],
				Style: Style{Size: 15},
				Width: 600,
			}
		}
		coldP := variants[0]

		b.Run("cold/"+tc.name, func(b *testing.B) {
			s := Shared()
			s.ReleaseShaping()
			b.ReportAllocs()
			for b.Loop() {
				s.ReleaseShaping()
				s.mu.Lock()
				s.layout(coldP)
				s.mu.Unlock()
			}
		})

		b.Run("edit/"+tc.name, func(b *testing.B) {
			s := Shared()
			s.ReleaseShaping()
			s.mu.Lock()
			s.layout(variants[15])
			s.mu.Unlock()
			s.EndFrame()
			var i int
			b.ReportAllocs()
			for b.Loop() {
				s.mu.Lock()
				s.layout(variants[i&15])
				s.mu.Unlock()
				s.EndFrame()
				i++
			}
		})
	}
}

func BenchmarkReleaseShapingIdleShrink(b *testing.B) {
	s := newSystem()
	b.ReportAllocs()
	var afterBytes int
	for b.Loop() {
		for s.MaskAtlas.Grow() {
		}
		s.ReleaseShaping()
		afterBytes = len(s.MaskAtlas.Pix) + len(s.ColorAtlas.Pix)
	}
	b.ReportMetric(float64(afterBytes)/1024, "idle-KiB")
}
