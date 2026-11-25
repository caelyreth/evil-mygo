package scene

import (
	"image"
	"math"
	"slices"
)

// A Differ finds where the successive scenes of a window differ, so that a
// renderer redraws only there: a blinking caret, a ticking clock or a
// button under the pointer redraws only itself.
//
// It compares the operations of two scenes from both ends, so a change in
// the middle of the display list damages the boxes of the operations that
// changed, clipped as they draw, where they drew before and where they
// draw now. Glyphs whose pixels changed in an atlas, and images whose
// pixels changed, count as changed.
type Differ struct {
	// Reach is how far, in pixels, a renderer draws beyond the geometry of
	// an operation: its antialiased edge, a pixel for the CPU's. Three more
	// are added under a transform, whose quads the GPU's renderers grow by
	// a pixel or two.
	Reach float32
	// Wide compares the colors outside the sRGB gamut (Scene.Wide) too, for
	// a renderer that draws them; others draw their nearest sRGB colors,
	// which the operations hold.
	Wide bool

	// valid tells that the last scene, of w×h pixels, is remembered to
	// compare the next with.
	valid   bool
	w, h    int
	clear   Color
	text    TextParams
	ops     []Op
	glyphs  []Glyph
	stops   []Stop
	effects []EffectOp
	wide    []WideColors
	words   []uint32
	// bounds is where each operation of the last scene drew, next where
	// those of the scene Diff was given draw; versions has the versions of
	// the last scene's images.
	bounds, next []image.Rectangle
	// encloses and encloses2 tell, for each clip of the last scene and of
	// the one Diff was given, that it holds an op drawn under a transform
	// that does more than move it, or a clip reaching into rounded corners
	// (see opBounds).
	encloses, encloses2 []bool
	pushes              []int
	corners             ClipCorners
	versions            []uint64
	fxVers              [][4]uint64
	mask, color         atlasMark
	damage              []image.Rectangle
	clips               []image.Rectangle
	// pre and suf are how many ops the scenes Diff compared last share at
	// their starts and ends, n and m how many each has (see Match).
	pre, suf, n, m int

	// Incremental tracking for Scene.Partial repaints.
	sceneID     uint64
	diffSceneID uint64
	clipOps     []int
	imgOps      []int
	fxImgOps    []int
	tmpOps      []int
	fast        bool
	allSame     bool
	fastEdit    PartEdit
	partBounds  []image.Rectangle
}

// atlasMark is the state of an atlas a scene drew from.
type atlasMark struct {
	atlas        *Atlas
	gen, version uint64
}

// changes returns the rectangles of a that changed since the mark, or
// false when everything may have.
func (m *atlasMark) changes(a *Atlas) ([]image.Rectangle, bool) {
	if a == nil || m.atlas == nil {
		return nil, a == m.atlas
	}
	if a != m.atlas {
		return nil, false
	}
	rects, full := a.Changes(m.gen, m.version)
	return rects, !full
}

func (m *atlasMark) set(a *Atlas) {
	m.atlas = a
	if a != nil {
		m.gen, m.version = a.Generation(), a.Version()
	}
}

// Invalidate makes the next Diff find that everything changed.
func (d *Differ) Invalidate() {
	d.valid = false
	d.sceneID = 0
	d.diffSceneID = 0
	d.fast = false
}

// ReuseUnchanged updates the tracked scene identity after Diff reported zero
// damage for a partial update of identical shape, keeping subsequent partial
// frames aligned without running Remember.
func (d *Differ) ReuseUnchanged(s *Scene) {
	if d.fast && d.allSame && s.ID != 0 && s.ID == d.diffSceneID {
		d.sceneID = s.ID
		d.fast = false
	}
}

// Diff compares s with the last scene Remember kept, and returns the
// rectangles where they differ, at most eight, apart from each other, or
// false when everything may have: none is kept, s is of another size, or
// starts with another color, or its atlases were reset. It finds where
// each operation of s draws as it goes (Bounds). The rectangles are valid
// until the next Diff.
func (d *Differ) Diff(s *Scene) ([]image.Rectangle, bool) {
	d.fast, d.allSame = false, false
	if damage, ok := d.tryPartialDiff(s); ok {
		return damage, true
	}
	d.next = d.opBounds(s, d.next[:0])
	d.pre, d.suf, d.n, d.m = 0, 0, 0, len(s.Ops)
	if !d.valid || s.Width != d.w || s.Height != d.h || s.Clear != d.clear || s.Text != d.text {
		return nil, false
	}
	maskRects, ok := d.mask.changes(s.MaskAtlas)
	if !ok {
		return nil, false
	}
	colorRects, ok := d.color.changes(s.ColorAtlas)
	if !ok {
		return nil, false
	}
	same := func(i, j int) bool { return d.same(i, s, j, maskRects, colorRects) }
	n, m := len(d.ops), len(s.Ops)
	pre := 0
	for pre < n && pre < m && same(pre, pre) {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && same(n-1-suf, m-1-suf) {
		suf++
	}
	d.pre, d.suf, d.n, d.m = pre, suf, n, m
	d.damage = d.damage[:0]
	if n == m {
		for k := pre; k < n-suf; k++ {
			if !same(k, k) {
				d.damage = addRect(d.damage, d.bounds[k])
				d.damage = addRect(d.damage, d.next[k])
			}
		}
	} else {
		for i := pre; i < n-suf; i++ {
			d.damage = addRect(d.damage, d.bounds[i])
		}
		for j := pre; j < m-suf; j++ {
			d.damage = addRect(d.damage, d.next[j])
		}
	}
	return d.damage, true
}

// Match returns the op of the last scene that op j of the scene Diff was
// given last stands for, and false for none: the rest of the ops in the
// same place, when the scenes have as many, that Diff found the same, and
// those it compared as changed, whose places are in the damage already.
// A renderer drawing an op that Match finds differently from the last
// frame (with another pipeline, say) damages its bounds.
func (d *Differ) Match(j int) (int, bool) {
	switch {
	case j < d.pre:
		return j, true
	case j >= d.m-d.suf && j < d.m:
		return j - d.m + d.n, true
	case d.n == d.m && j < d.m:
		return j, true
	}
	return 0, false
}

// Bounds returns where each operation of the scene Diff was given last
// draws, within the clips around it.
func (d *Differ) Bounds() []image.Rectangle {
	if !d.fast {
		return d.next
	}
	pe := d.fastEdit
	n := pe.OpStart + len(d.partBounds) + (len(d.bounds) - pe.OldOpEnd)
	d.next = slices.Grow(d.next[:0], n)[:0]
	d.next = append(d.next, d.bounds[:pe.OpStart]...)
	d.next = append(d.next, d.partBounds...)
	d.next = append(d.next, d.bounds[pe.OldOpEnd:]...)
	return d.next
}

// OpBound returns the clipped bounds computed by the last Diff for op j,
// without materializing a full slice when Diff used the partial fast path.
func (d *Differ) OpBound(j int) image.Rectangle {
	if !d.fast {
		return d.next[j]
	}
	pe := d.fastEdit
	if j < pe.OpStart {
		return d.bounds[j]
	}
	if j < pe.NewOpEnd {
		return d.partBounds[j-pe.OpStart]
	}
	return d.bounds[j-pe.NewOpEnd+pe.OldOpEnd]
}

// RememberedBound returns the clipped bounds stored by the last Remember for
// op j.
func (d *Differ) RememberedBound(j int) image.Rectangle { return d.bounds[j] }

// AddBackdrops adds to damage, rectangles to redraw in s after Diff, the
// effects of s that read their backdrops it meets, with those backdrops,
// and returns it: an effect reads its backdrop after drawing the
// operations before it within the area drawn, which must hold all of it
// for the backdrop to hold none of what the frame drew before. So the
// damage becomes rectangles apart from each other.
func (d *Differ) AddBackdrops(s *Scene, damage []image.Rectangle) []image.Rectangle {
	if len(s.Effects) == 0 || len(damage) == 0 {
		return damage
	}
	for changed := true; changed; {
		changed = false
		for i := range s.Ops {
			op := &s.Ops[i]
			if op.Kind != OpEffect || int(op.Start) >= len(s.Effects) {
				continue
			}
			bound := d.OpBound(i)
			if bound.Empty() {
				continue
			}
			fx := &s.Effects[op.Start]
			if fx.Effect == nil || !fx.Effect.Backdrop {
				continue
			}
			need := s.BackdropOf(op.Transform.Bounds(op.Rect), fx.Blur, s.Width, s.Height).Area.Union(bound)
			for _, r := range damage {
				if r.Overlaps(need) && !need.In(r) {
					damage = addRect(damage, need)
					changed = true
					break
				}
			}
		}
		// Rectangles that overlap become one.
		for i := 0; i < len(damage); i++ {
			for j := i + 1; j < len(damage); j++ {
				if damage[i].Overlaps(damage[j]) {
					damage[i] = damage[i].Union(damage[j])
					damage = append(damage[:j], damage[j+1:]...)
					changed = true
					j = i
				}
			}
		}
	}
	return damage
}

// same reports whether operation i of the last scene draws what operation
// j of s does.
func (d *Differ) same(i int, s *Scene, j int, maskRects, colorRects []image.Rectangle) bool {
	a, b := d.ops[i], s.Ops[j]
	// Glyphs and effects are compared by what Start and End point at.
	sa, sb := a.Start, b.Start
	ga, gb := d.glyphs[:0], s.Glyphs[:0]
	if a.Kind == OpGlyphs && b.Kind == OpGlyphs {
		ga, gb = d.glyphs[a.Start:a.End], s.Glyphs[b.Start:b.End]
	} else if a.Kind == OpEffect && b.Kind == OpEffect && int(sa) < len(d.effects) && int(sb) < len(s.Effects) {
		// A shader's effect filling mask glyphs.
		x, y := d.effects[sa].Glyphs, s.Effects[sb].Glyphs
		ga, gb = d.glyphs[x[0]:x[1]], s.Glyphs[y[0]:y[1]]
	}
	a.Start, a.End, b.Start, b.End = 0, 0, 0, 0
	if a.Kind == OpPushClip && d.encloses[i] != d.encloses2[j] {
		return false
	}
	// Gradients' stops are compared by what Stops point at.
	if !sameStops(d.stops, a.Stops, s.Stops, b.Stops) {
		return false
	}
	a.Stops, b.Stops = [2]int32{}, [2]int32{}
	// The colors outside the sRGB gamut are compared by what Wide points
	// at in each scene's own table.
	wa, wb := a.Wide, b.Wide
	a.Wide, b.Wide = 0, 0
	if a != b || !d.sameWide(wa, s, wb) {
		return false
	}
	if !d.sameGlyphs(ga, s, gb, maskRects, colorRects) {
		return false
	}
	switch b.Kind {
	case OpImage, OpPushClip:
		// A clip's image masks it.
		if b.Image != nil && b.Image.Version() != d.versions[i] {
			return false
		}
	case OpEffect:
		if int(sa) >= len(d.effects) || int(sb) >= len(s.Effects) || !d.effects[sa].Same(&s.Effects[sb]) {
			return false
		}
		for k, img := range s.Effects[sb].Images {
			if img != nil && img.Version() != d.fxVers[sa][k] {
				return false
			}
		}
	}
	return true
}

// sameWide reports whether the colors outside the sRGB gamut that wa
// points at in the last scene are those wb does in s, when they are drawn.
func (d *Differ) sameWide(wa uint16, s *Scene, wb uint16) bool {
	if !d.Wide || wa == 0 && wb == 0 {
		return true
	}
	if wa == 0 || wb == 0 || int(wa) > len(d.wide) || int(wb) > len(s.Wide) {
		return false
	}
	return d.wide[wa-1] == s.Wide[wb-1]
}

// sameStops reports whether stops sa[a[0]:a[1]] of the last scene are
// those sb[b[0]:b[1]] of the new one.
func sameStops(sa []Stop, a [2]int32, sb []Stop, b [2]int32) bool {
	if a[1]-a[0] != b[1]-b[0] {
		return false
	}
	if a[1] <= a[0] {
		return true
	}
	if int(a[1]) > len(sa) || int(b[1]) > len(sb) || a[0] < 0 || b[0] < 0 {
		return false
	}
	for i := range a[1] - a[0] {
		if sa[a[0]+i] != sb[b[0]+i] {
			return false
		}
	}
	return true
}

// sameGlyphs reports whether glyphs ga of the last scene draw what gb of
// s do, none of them in an area of the atlases that changed.
func (d *Differ) sameGlyphs(ga []Glyph, s *Scene, gb []Glyph, maskRects, colorRects []image.Rectangle) bool {
	if len(ga) != len(gb) {
		return false
	}
	for k := range gb {
		g := &gb[k]
		if x := ga[k]; x != *g {
			wa := x.Wide
			x.Wide = g.Wide
			if x != *g || !d.sameWide(wa, s, g.Wide) {
				return false
			}
		} else if d.Wide && g.Wide != 0 && !d.sameWide(g.Wide, s, g.Wide) {
			return false
		}
		rects := maskRects
		if g.Colored {
			rects = colorRects
		}
		at := image.Rect(int(g.U), int(g.V), int(g.U)+int(g.UW), int(g.V)+int(g.VH))
		for _, c := range rects {
			if c.Overlaps(at) {
				return false
			}
		}
	}
	return true
}

// addRect adds a rectangle to damage, merged with one it touches.
func addRect(damage []image.Rectangle, b image.Rectangle) []image.Rectangle {
	if b.Empty() {
		return damage
	}
	for i, d := range damage {
		if d.Inset(-8).Overlaps(b) {
			damage[i] = d.Union(b)
			return damage
		}
	}
	damage = append(damage, b)
	if len(damage) > 8 {
		u := damage[0]
		for _, d := range damage[1:] {
			u = u.Union(d)
		}
		damage = append(damage[:0], u)
	}
	return damage
}

func (d *Differ) reachOf(m Affine) float32 {
	if m.Set {
		return d.Reach + 3
	}
	return d.Reach
}

// opBound returns the unclipped pixel bounds of a single non-clip op of s.
func (d *Differ) opBound(s *Scene, op *Op) image.Rectangle {
	switch op.Kind {
	case OpFill, OpImage, OpEffect, OpHole:
		return outset(op.Transform.Bounds(op.Rect), d.reachOf(op.Transform))
	case OpShadow:
		if op.Inset {
			return outset(op.Transform.Bounds(op.Cast), d.reachOf(op.Transform))
		}
		e := 1.5*op.Blur + 1
		box := Rect{X: op.Rect.X - e, Y: op.Rect.Y - e, W: op.Rect.W + 2*e, H: op.Rect.H + 2*e}
		return outset(op.Transform.Bounds(box), d.reachOf(op.Transform))
	case OpGlyphs:
		var b image.Rectangle
		// The glyphs without transforms of their own as one box, which
		// the op's transform maps, the others each.
		x0, y0, x1, y1 := float32(math.MaxFloat32), float32(math.MaxFloat32), float32(-math.MaxFloat32), float32(-math.MaxFloat32)
		for _, g := range s.Glyphs[op.Start:op.End] {
			// The GPU's renderers draw the bitmap's own size, at a
			// whole pixel.
			w, h := max(g.W, float32(g.UW)), max(g.H, float32(g.VH))
			if !g.Transform.Set {
				x0, y0, x1, y1 = min(x0, g.X), min(y0, g.Y), max(x1, g.X+w), max(y1, g.Y+h)
				continue
			}
			m := op.Transform.Mul(g.Transform)
			b = b.Union(outset(m.Bounds(Rect{X: g.X, Y: g.Y, W: w, H: h}), d.reachOf(m)))
		}
		switch {
		case x0 > x1:
		case op.Transform.Set:
			b = b.Union(outset(op.Transform.Bounds(Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}), d.reachOf(op.Transform)))
		default:
			b = b.Union(outsetEdges(x0, y0, x1, y1, d.Reach))
		}
		return b
	}
	return image.Rectangle{}
}

func (d *Differ) tryPartialDiff(s *Scene) ([]image.Rectangle, bool) {
	pe := s.Partial
	if !d.valid || pe.BaseID == 0 || pe.BaseID != d.sceneID ||
		s.Width != d.w || s.Height != d.h || s.Clear != d.clear || s.Text != d.text {
		return nil, false
	}
	n, m := len(d.ops), len(s.Ops)
	if pe.OpStart < 0 || pe.OpStart > pe.OldOpEnd || pe.OldOpEnd > n ||
		pe.NewOpEnd < pe.OpStart || pe.NewOpEnd > m ||
		m-pe.NewOpEnd != n-pe.OldOpEnd {
		return nil, false
	}
	if pe.GlyphMark < 0 || pe.GlyphMark > len(d.glyphs) || pe.GlyphMark > len(s.Glyphs) ||
		pe.StopMark < 0 || pe.StopMark > len(d.stops) || pe.StopMark > len(s.Stops) ||
		pe.EffectMark < 0 || pe.EffectMark > len(d.effects) || pe.EffectMark > len(s.Effects) ||
		pe.WideMark < 0 || pe.WideMark > len(s.Wide) || (d.Wide && pe.WideMark > len(d.wide)) {
		return nil, false
	}
	if maskRects, ok := d.mask.changes(s.MaskAtlas); !ok || len(maskRects) > 0 {
		return nil, false
	}
	if colorRects, ok := d.color.changes(s.ColorAtlas); !ok || len(colorRects) > 0 {
		return nil, false
	}
	for _, idx := range d.imgOps {
		if idx >= pe.OpStart && idx < pe.OldOpEnd {
			continue
		}
		if img := d.ops[idx].Image; img != nil && img.Version() != d.versions[idx] {
			return nil, false
		}
	}
	for _, idx := range d.fxImgOps {
		if idx >= pe.OpStart && idx < pe.OldOpEnd {
			continue
		}
		sa := d.ops[idx].Start
		for k, img := range d.effects[sa].Images {
			if img != nil && img.Version() != d.fxVers[sa][k] {
				return nil, false
			}
		}
	}
	prevScene := Scene{Glyphs: d.glyphs, Effects: d.effects}
	for i := pe.OpStart; i < pe.OldOpEnd; i++ {
		op := &d.ops[i]
		if op.Kind == OpPushClip || op.Kind == OpPopClip || Reshaped(&prevScene, op) {
			return nil, false
		}
	}
	for j := pe.OpStart; j < pe.NewOpEnd; j++ {
		op := &s.Ops[j]
		if op.Kind == OpPushClip || op.Kind == OpPopClip || Reshaped(s, op) {
			return nil, false
		}
	}
	clip := image.Rect(0, 0, s.Width, s.Height)
	d.clips = d.clips[:0]
	for _, idx := range d.clipOps {
		if idx >= pe.OpStart {
			break
		}
		if d.ops[idx].Kind == OpPushClip {
			d.clips = append(d.clips, clip)
			clip = d.bounds[idx]
		} else if len(d.clips) > 0 {
			clip = d.clips[len(d.clips)-1]
			d.clips = d.clips[:len(d.clips)-1]
		}
	}
	need := pe.NewOpEnd - pe.OpStart
	d.partBounds = slices.Grow(d.partBounds[:0], need)[:0]
	for j := pe.OpStart; j < pe.NewOpEnd; j++ {
		b := d.opBound(s, &s.Ops[j])
		d.partBounds = append(d.partBounds, b.Intersect(clip))
	}
	d.pre, d.suf, d.n, d.m = pe.OpStart, n-pe.OldOpEnd, n, m
	d.damage = d.damage[:0]
	allSame := pe.OldOpEnd == pe.NewOpEnd &&
		len(d.glyphs) == len(s.Glyphs) && len(d.stops) == len(s.Stops) &&
		len(d.effects) == len(s.Effects) && (!d.Wide || len(d.wide) == len(s.Wide))
	if pe.OldOpEnd == pe.NewOpEnd {
		for k := pe.OpStart; k < pe.OldOpEnd; k++ {
			if !d.same(k, s, k, nil, nil) {
				allSame = false
				d.damage = addRect(d.damage, d.bounds[k])
				d.damage = addRect(d.damage, d.partBounds[k-pe.OpStart])
			}
		}
	} else {
		for i := pe.OpStart; i < pe.OldOpEnd; i++ {
			d.damage = addRect(d.damage, d.bounds[i])
		}
		for j := pe.OpStart; j < pe.NewOpEnd; j++ {
			d.damage = addRect(d.damage, d.partBounds[j-pe.OpStart])
		}
	}
	d.fast = true
	d.allSame = allSame
	d.fastEdit = pe
	d.diffSceneID = s.ID
	return d.damage, true
}

// opBounds appends where each operation of s draws, within the clips
// around it.
func (d *Differ) opBounds(s *Scene, out []image.Rectangle) []image.Rectangle {
	clip := image.Rect(0, 0, s.Width, s.Height)
	d.clips, d.pushes = d.clips[:0], d.pushes[:0]
	d.corners.Reset()
	d.encloses2 = slices.Grow(d.encloses2[:0], len(s.Ops))[:len(s.Ops)]
	clear(d.encloses2)
	for i := range s.Ops {
		op := &s.Ops[i]
		var b image.Rectangle
		corners := false
		switch op.Kind {
		case OpPushClip:
			// A clip that changes changes everything it cuts.
			b = outset(op.Transform.Bounds(op.Rect), d.reachOf(op.Transform))
			d.clips = append(d.clips, clip)
			d.pushes = append(d.pushes, i)
			clip = clip.Intersect(b)
			corners = d.corners.Push(op) >= 0
		case OpPopClip:
			if len(d.clips) > 0 {
				clip = d.clips[len(d.clips)-1]
				d.clips = d.clips[:len(d.clips)-1]
				d.pushes = d.pushes[:len(d.pushes)-1]
				d.corners.Pop()
			}
		default:
			b = d.opBound(s, op)
		}
		// The GPU's renderers cut what a clip holds with a mask once an op
		// in it is drawn under a transform that does more than move it
		// (Reshaped), and in the shader otherwise, which differ at its
		// edge by a unit in the last place: a clip that starts or stops
		// holding one changes all it cuts. A clip pushed under such a
		// transform is one for the clips around it, not for itself, which
		// it was pushed onto already (as gpu.Builder.markEnclosing has it).
		// So does a clip that starts or stops holding one that reaches
		// into its rounded corners, or those of a clip it is in
		// (ClipCorners).
		if corners || op.Kind != OpPopClip && Reshaped(s, op) {
			j := len(d.pushes) - 1
			if op.Kind == OpPushClip {
				j--
			}
			for ; j >= 0 && !d.encloses2[d.pushes[j]]; j-- {
				d.encloses2[d.pushes[j]] = true
			}
		}
		out = append(out, b.Intersect(clip))
	}
	return out
}

// outsetEdges returns the pixels the box from x0, y0 to x1, y1 grown by d
// touches.
func outsetEdges(x0, y0, x1, y1, d float32) image.Rectangle {
	return image.Rect(int(math.Floor(float64(x0-d))), int(math.Floor(float64(y0-d))),
		int(math.Ceil(float64(x1+d))), int(math.Ceil(float64(y1+d))))
}

func outset(rc Rect, d float32) image.Rectangle {
	return image.Rect(int(math.Floor(float64(rc.X-d))), int(math.Floor(float64(rc.Y-d))),
		int(math.Ceil(float64(rc.X+rc.W+d))), int(math.Ceil(float64(rc.Y+rc.H+d))))
}

// Remember keeps what Diff needs of s, which Diff was given last, to
// compare the next scene with.
func (d *Differ) Remember(s *Scene) {
	if d.fast && s.ID != 0 && s.ID == d.diffSceneID && s.Partial == d.fastEdit && s.Partial.BaseID == d.sceneID {
		d.rememberPartial(s)
		return
	}
	d.fast, d.allSame = false, false
	d.valid = true
	d.sceneID = s.ID
	d.w, d.h = s.Width, s.Height
	d.clear, d.text = s.Clear, s.Text
	if len(d.ops) > len(s.Ops) {
		clear(d.ops[len(s.Ops):])
	}
	d.ops = append(d.ops[:0], s.Ops...)
	d.glyphs = append(d.glyphs[:0], s.Glyphs...)
	d.stops = append(d.stops[:0], s.Stops...)
	d.wide = d.wide[:0]
	if d.Wide {
		d.wide = append(d.wide, s.Wide...)
	}
	if len(d.effects) > len(s.Effects) {
		clear(d.effects[len(s.Effects):])
	}
	d.effects = append(d.effects[:0], s.Effects...)
	d.sliceEffectWords(0, 0)
	d.bounds, d.next = d.next, d.bounds
	d.encloses, d.encloses2 = d.encloses2, d.encloses
	d.versions = d.versions[:0]
	d.clipOps = d.clipOps[:0]
	d.imgOps = d.imgOps[:0]
	d.fxImgOps = d.fxImgOps[:0]
	for i := range s.Ops {
		op := &s.Ops[i]
		v := uint64(0)
		if img := op.Image; img != nil {
			v = img.Version()
			d.imgOps = append(d.imgOps, i)
		}
		d.versions = append(d.versions, v)
		if op.Kind == OpPushClip || op.Kind == OpPopClip {
			d.clipOps = append(d.clipOps, i)
		} else if hasEffectImage(s, op) {
			d.fxImgOps = append(d.fxImgOps, i)
		}
	}
	d.fxVers = d.fxVers[:0]
	for i := range s.Effects {
		d.fxVers = append(d.fxVers, effectVersions(&s.Effects[i]))
	}
	d.mask.set(s.MaskAtlas)
	d.color.set(s.ColorAtlas)
}

func hasEffectImage(s *Scene, op *Op) bool {
	if op.Kind != OpEffect || op.Start < 0 || int(op.Start) >= len(s.Effects) {
		return false
	}
	for _, img := range s.Effects[op.Start].Images {
		if img != nil {
			return true
		}
	}
	return false
}

func effectVersions(fx *EffectOp) (vs [4]uint64) {
	for k, img := range fx.Images {
		if img != nil {
			vs[k] = img.Version()
		}
	}
	return vs
}

func (d *Differ) sliceEffectWords(start, at int) {
	if start == 0 {
		d.words = d.words[:0]
		at = 0
	} else {
		d.words = d.words[:at]
	}
	oldCap := cap(d.words)
	for i := start; i < len(d.effects); i++ {
		d.words = append(d.words, d.effects[i].Uniforms...)
		d.words = append(d.words, d.effects[i].WideUniforms...)
		d.words = append(d.words, d.effects[i].Instances...)
	}
	if start > 0 && cap(d.words) != oldCap {
		start, at = 0, 0
	}
	for i := start; i < len(d.effects); i++ {
		fx := &d.effects[i]
		n, w, q := len(fx.Uniforms), len(fx.WideUniforms), len(fx.Instances)
		fx.Uniforms = d.words[at : at+n : at+n]
		if fx.WideUniforms != nil {
			fx.WideUniforms = d.words[at+n : at+n+w : at+n+w]
		}
		if fx.Instances != nil {
			fx.Instances = d.words[at+n+w : at+n+w+q : at+n+w+q]
		}
		at += n + w + q
	}
}

func (d *Differ) rememberPartial(s *Scene) {
	pe := s.Partial
	d.fast, d.allSame = false, false
	d.valid = true
	d.sceneID = s.ID
	delta := pe.NewOpEnd - pe.OldOpEnd
	if delta == 0 {
		copy(d.ops[pe.OpStart:pe.NewOpEnd], s.Ops[pe.OpStart:pe.NewOpEnd])
		copy(d.bounds[pe.OpStart:pe.NewOpEnd], d.partBounds)
		clear(d.encloses[pe.OpStart:pe.NewOpEnd])
	} else {
		oldLen, newLen := len(d.ops), len(s.Ops)
		if delta > 0 {
			d.ops = slices.Grow(d.ops, delta)[:newLen]
			d.bounds = slices.Grow(d.bounds, delta)[:newLen]
			d.encloses = slices.Grow(d.encloses, delta)[:newLen]
			d.versions = slices.Grow(d.versions, delta)[:newLen]
		}
		copy(d.ops[pe.NewOpEnd:newLen], d.ops[pe.OldOpEnd:oldLen])
		copy(d.bounds[pe.NewOpEnd:newLen], d.bounds[pe.OldOpEnd:oldLen])
		copy(d.encloses[pe.NewOpEnd:newLen], d.encloses[pe.OldOpEnd:oldLen])
		copy(d.versions[pe.NewOpEnd:newLen], d.versions[pe.OldOpEnd:oldLen])
		if delta < 0 {
			clear(d.ops[newLen:])
			d.ops = d.ops[:newLen]
			d.bounds = d.bounds[:newLen]
			d.encloses = d.encloses[:newLen]
			d.versions = d.versions[:newLen]
		}
		copy(d.ops[pe.OpStart:pe.NewOpEnd], s.Ops[pe.OpStart:pe.NewOpEnd])
		copy(d.bounds[pe.OpStart:pe.NewOpEnd], d.partBounds)
		clear(d.encloses[pe.OpStart:pe.NewOpEnd])
		for k := range d.clipOps {
			if d.clipOps[k] >= pe.OldOpEnd {
				d.clipOps[k] += delta
			}
		}
	}
	for j := pe.OpStart; j < pe.NewOpEnd; j++ {
		v := uint64(0)
		if img := s.Ops[j].Image; img != nil {
			v = img.Version()
		}
		d.versions[j] = v
	}
	d.glyphs = append(d.glyphs[:pe.GlyphMark], s.Glyphs[pe.GlyphMark:]...)
	d.stops = append(d.stops[:pe.StopMark], s.Stops[pe.StopMark:]...)
	if d.Wide {
		d.wide = append(d.wide[:pe.WideMark], s.Wide[pe.WideMark:]...)
	} else {
		d.wide = d.wide[:0]
	}
	if pe.EffectMark < len(s.Effects) || pe.EffectMark < len(d.effects) {
		wordAt := 0
		for i := 0; i < pe.EffectMark; i++ {
			fx := &d.effects[i]
			wordAt += len(fx.Uniforms) + len(fx.WideUniforms) + len(fx.Instances)
		}
		if len(d.effects) > len(s.Effects) {
			clear(d.effects[len(s.Effects):])
		}
		d.effects = append(d.effects[:pe.EffectMark], s.Effects[pe.EffectMark:]...)
		d.sliceEffectWords(pe.EffectMark, wordAt)
		d.fxVers = d.fxVers[:pe.EffectMark]
		for i := pe.EffectMark; i < len(s.Effects); i++ {
			d.fxVers = append(d.fxVers, effectVersions(&s.Effects[i]))
		}
	}
	d.imgOps = d.spliceOpIndex(d.imgOps, pe, delta, func(j int) bool { return s.Ops[j].Image != nil })
	d.fxImgOps = d.spliceOpIndex(d.fxImgOps, pe, delta, func(j int) bool { return hasEffectImage(s, &s.Ops[j]) })
	d.mask.set(s.MaskAtlas)
	d.color.set(s.ColorAtlas)
}

func (d *Differ) spliceOpIndex(list []int, pe PartEdit, delta int, match func(int) bool) []int {
	d.tmpOps = d.tmpOps[:0]
	for _, idx := range list {
		if idx < pe.OpStart {
			d.tmpOps = append(d.tmpOps, idx)
		}
	}
	for j := pe.OpStart; j < pe.NewOpEnd; j++ {
		if match(j) {
			d.tmpOps = append(d.tmpOps, j)
		}
	}
	for _, idx := range list {
		if idx >= pe.OldOpEnd {
			d.tmpOps = append(d.tmpOps, idx+delta)
		}
	}
	return append(list[:0], d.tmpOps...)
}

// Release forgets the last scene and lets go of what it kept.
func (d *Differ) Release() {
	reach, wide := d.Reach, d.Wide
	*d = Differ{Reach: reach, Wide: wide}
}
