package ui

import (
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/text"
)

// Drawing is what a view painted, kept as vectors rather than as the
// frame's pixels: rectangles, paths and glyphs where the frame rasterizes
// them, for a target that draws them itself, as a PDF does. Record makes
// one.
//
// Items stand in the order painted. A clip holds from its DrawPushClip to
// the matching DrawPopClip. What has no vector form is left out: shadows,
// effects, holes and gradients' second colors (a gradient fills with its
// first).
type Drawing struct {
	Width, Height float32
	Items         []DrawItem
	// outlines are the glyphs' outlines, read while the fonts were the
	// recording's to read.
	outlines map[glyphKey]*Path
}

type glyphKey struct {
	f  *text.Font
	id uint32
}

// Outline returns the outline of glyph id of f, as the drawing drew it,
// in DIPs from its origin on the baseline, y down; nil where the
// platform's text system gives none, or the glyph is blank.
func (d *Drawing) Outline(f GlyphFont, id uint32) *Path { return d.outlines[glyphKey{f.f, id}] }

// DrawKind is the kind of a DrawItem.
type DrawKind uint8

const (
	// DrawFill fills Rect, rounded by Radii (clockwise from the top left),
	// with Color, and inside its edges a border of Border widths (top,
	// right, bottom, left) in BorderColor.
	DrawFill DrawKind = iota
	// DrawImage draws Image's Src into Rect.
	DrawImage
	// DrawPushClip clips what follows to Rect with Radii.
	DrawPushClip
	// DrawPopClip ends the innermost clip.
	DrawPopClip
	// DrawPath fills Path with Color or, where Width is positive, strokes
	// it that wide.
	DrawPath
	// DrawGlyphs draws Glyphs, each in its own color.
	DrawGlyphs
)

// DrawItem is one thing a Drawing holds; which fields matter depends on
// Kind. Its geometry is in DIPs, mapped by Transform.
type DrawItem struct {
	Kind DrawKind
	// Transform maps the item's geometry to the drawing's: x' = A x + C y
	// + X, y' = B x + D y + Y.
	Transform DrawTransform
	Rect      Rect
	Radii     [4]float32
	// Color is straight, its alpha times the painter's opacity.
	Color       Color
	Border      [4]float32
	BorderColor Color
	Path        *Path
	Width       float32
	Image       *DrawImageData
	Src         Rect
	Glyphs      []DrawGlyph
}

// DrawTransform is an affine map; the zero value is the identity.
type DrawTransform struct {
	A, B, C, D, X, Y float32
	Set              bool
}

// DrawImageData is an image's pixels: premultiplied RGBA rows of 4*W
// bytes.
type DrawImageData struct {
	W, H int
	Pix  []byte
	// ID is the same for the same pixels, so a target keeps one copy.
	ID uint64
}

// DrawGlyph is a glyph on its baseline at (X, Y), of Font, in Color,
// moving the pen Advance on. Text is what its cluster spells, on the
// glyph that starts the cluster and empty on the others.
type DrawGlyph struct {
	Font    GlyphFont
	ID      uint32
	X, Y    float32
	Advance float32
	Color   Color
	Text    string
}

// GlyphFont is a font glyphs are drawn in; equal values are one font at
// one size.
type GlyphFont struct{ f *text.Font }

// Size is the font's em size in DIPs.
func (f GlyphFont) Size() float32 { return f.f.Size }

// outline reads the outline of the glyph id at the font's size.
func (f GlyphFont) outline(id uint32) *Path {
	segs := text.GlyphPath(f.f, id)
	if len(segs) == 0 {
		return nil
	}
	p := &Path{}
	for _, s := range segs {
		pt := func(i int) (float32, float32) { return s.Pts[i][0], -s.Pts[i][1] }
		switch s.Op {
		case text.PathMove:
			p.MoveTo(pt(0))
		case text.PathLine:
			p.LineTo(pt(0))
		case text.PathQuad:
			cx, cy := pt(0)
			x, y := pt(1)
			p.QuadTo(cx, cy, x, y)
		case text.PathCubic:
			c1x, c1y := pt(0)
			c2x, c2y := pt(1)
			x, y := pt(2)
			p.CubeTo(c1x, c1y, c2x, c2y, x, y)
		case text.PathClose:
			p.Close()
		}
	}
	return p
}

// PathOp is the kind of a step of a Path.
type PathOp uint8

const (
	PathMoveTo PathOp = iota
	PathLineTo
	PathQuadTo
	PathCubeTo
	PathClose
)

// Each calls fn with each step of the path in order and its points: the
// end point last, after a curve's control points.
func (p *Path) Each(fn func(op PathOp, pts [][2]float32)) {
	for i := range p.cmds {
		c := &p.cmds[i]
		n := [...]int{1, 1, 2, 3, 0}[c.op]
		fn(PathOp(c.op), c.pts[:n])
	}
}

// EvenOdd reports whether the path fills by the even-odd rule.
func (p *Path) EvenOdd() bool { return p.rule == EvenOdd }

// Record paints a frame of view in a window of width×height DIPs, without
// a window, and returns what it painted as a Drawing. Motion is reduced,
// so what animates stands where it ends.
func Record(view func(c *Context), width, height int) *Drawing {
	d := &Drawing{Width: float32(width), Height: float32(height)}
	t := coreNewTesterWith(func(c *context) { view(makeContext(c)) }, width, height, TesterOptions{Preferences: Preferences{ReduceMotion: true}})
	t.rt.record = d
	t.Frame()
	// The fonts are the Tester's text system's, which goes back to be
	// used again as it ends: their outlines are read now.
	d.outlines = map[glyphKey]*Path{}
	for _, it := range d.Items {
		for _, g := range it.Glyphs {
			k := glyphKey{g.Font.f, g.ID}
			if _, ok := d.outlines[k]; !ok {
				d.outlines[k] = g.Font.outline(g.ID)
			}
		}
	}
	t.end()
	return d
}

// recording reports whether the painter keeps a Drawing.
func (p *Painter) recording() bool { return p.rec != nil }

func (p *Painter) recordTransform(extra scene.Affine) DrawTransform {
	a := extra
	if p.transform.Set {
		a = p.transform.Mul(extra)
	}
	if !a.Set {
		return DrawTransform{}
	}
	return DrawTransform{A: a.A, B: a.B, C: a.C, D: a.D, X: a.X, Y: a.Y, Set: true}
}

func fromScene(c scene.Color) Color { return Color{R: c.R, G: c.G, B: c.B, A: c.A} }

func opacityOf(o float32) float32 {
	if o == 0 {
		return 1
	}
	return o
}

// recordOp keeps the op add was given, where it has a vector form.
func (p *Painter) recordOp(op scene.Op) {
	it := DrawItem{Transform: p.recordTransform(op.Transform), Rect: Rect(op.Rect), Radii: op.Radii}
	alpha := opacityOf(op.Opacity)
	switch op.Kind {
	case scene.OpFill:
		it.Kind = DrawFill
		it.Color = fromScene(op.Color).Alpha(alpha)
		if op.Paint != scene.PaintSolid && op.Paint.HasStops() && p.s != nil && op.Stops[1] > op.Stops[0] {
			it.Color = fromScene(p.s.Stops[op.Stops[0]].Color).Alpha(alpha)
		}
		if scene.HasBorder(op.Border) {
			it.Border, it.BorderColor = op.Border, fromScene(op.BorderColor).Alpha(alpha)
		}
	case scene.OpImage:
		img := op.Image
		if img == nil || img.Alpha || len(img.Pix) < 4*img.W*img.H {
			return
		}
		it.Kind = DrawImage
		it.Image = &DrawImageData{W: img.W, H: img.H, Pix: img.Pix, ID: img.ID()}
		it.Src = Rect(op.Src)
		it.Color = Color{A: uint8(255 * alpha)}
	case scene.OpPushClip:
		it.Kind = DrawPushClip
	case scene.OpPopClip:
		it.Kind = DrawPopClip
	default:
		return
	}
	p.rec.Items = append(p.rec.Items, it)
}

// recordPath keeps path filled, or stroked width wide, in c.
func (p *Painter) recordPath(path *Path, x, y, width float32, c Color) {
	kept := &Path{cmds: append([]pathCmd(nil), path.cmds...), rule: path.rule, cap: path.cap, join: path.join}
	it := DrawItem{Kind: DrawPath, Transform: p.recordTransform(scene.Matrix(1, 0, 0, 1, x, y)), Path: kept, Width: width, Color: c.Alpha(p.opacity)}
	p.rec.Items = append(p.rec.Items, it)
}

// recordLine keeps the glyphs of a line of l laid out from (x, y), each in
// the color its span gives it.
func (p *Painter) recordLine(l *text.Layout, line *text.Line, x, y float32, color Color, sp *spanPaint) {
	it := DrawItem{Kind: DrawGlyphs, Transform: p.recordTransform(scene.Affine{})}
	runes := []rune(l.Params.Text)
	for i, g := range line.Glyphs {
		if g.Object {
			continue
		}
		c := color
		if sp != nil {
			if c = sp.color(sp.at(g.Cluster), color); c.A == 0 {
				continue
			}
		}
		dg := DrawGlyph{Font: GlyphFont{g.Font}, ID: g.ID, X: x + g.X, Y: y + g.Y, Advance: g.Advance, Color: c.Alpha(p.opacity)}
		if i == 0 || line.Glyphs[i-1].Cluster != g.Cluster {
			if end := g.Cluster + g.Runes; g.Runes > 0 && g.Cluster >= 0 && end <= len(runes) {
				dg.Text = string(runes[g.Cluster:end])
			}
		}
		it.Glyphs = append(it.Glyphs, dg)
	}
	if len(it.Glyphs) > 0 {
		p.rec.Items = append(p.rec.Items, it)
	}
}
