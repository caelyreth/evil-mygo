package ui

import (
	"runtime"
	"testing"
)

// What a view paints comes back as vectors: its fills, and its text as
// glyphs that know their font and what they spell.
func TestRecordKeepsFillsAndGlyphs(t *testing.T) {
	d := Record(func(c *Context) {
		Column(c).Fill().Background(Color{R: 250, G: 250, B: 250, A: 255}).Children(func() {
			Text(c, "Tide")
		})
	}, 200, 100)
	var fills, glyphs int
	var text string
	var glyph DrawGlyph
	for _, it := range d.Items {
		switch it.Kind {
		case DrawFill:
			fills++
		case DrawGlyphs:
			for _, g := range it.Glyphs {
				glyphs++
				text += g.Text
				glyph = g
			}
		}
	}
	if fills == 0 || glyphs != 4 || text != "Tide" {
		t.Fatalf("fills %d, glyphs %d spelling %q", fills, glyphs, text)
	}
	if runtime.GOOS == "darwin" {
		if p := d.Outline(glyph.Font, glyph.ID); p == nil || glyph.Font.Size() <= 0 {
			t.Fatal("a glyph has no outline")
		}
	}
}
