package scene

import (
	"image"
	"math"
	"slices"
	"testing"
)

// TestDamageOfNestedTransformedClip checks that a clip under a
// transform that turns it, nested in a rounded one, changes all the
// rounded one cuts as it comes and goes: the GPU's renderers then cut the
// outer clip with a mask rather than in the shader, at its edges a unit in
// the last place apart. A clip only moved changes nothing around it, as
// the renderers cut it as any other.
func TestDamageOfNestedTransformedClip(t *testing.T) {
	outer := Op{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 150, H: 150}, Radii: Uniform(8)}
	corner := Op{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 20, H: 20}, Color: Color{R: 255, A: 255}}
	nested := []Op{
		{Kind: OpPushClip, Rect: Rect{W: 40, H: 40}, Transform: Translation(100, 100).Mul(Rotation(10))},
		{Kind: OpFill, Rect: Rect{X: 100, Y: 100, W: 20, H: 20}, Color: Color{G: 255, A: 255}},
		{Kind: OpPopClip},
	}
	without := &Scene{Width: 200, Height: 200, Ops: []Op{outer, corner, {Kind: OpPopClip}}}
	with := &Scene{Width: 200, Height: 200, Ops: append(append([]Op{outer, corner}, nested...), Op{Kind: OpPopClip})}
	for _, frames := range [][2]*Scene{{without, with}, {with, without}} {
		d := Differ{Reach: 2}
		d.Diff(frames[0])
		d.Remember(frames[0])
		damage, ok := d.Diff(frames[1])
		if !ok {
			t.Fatal("no damage found")
		}
		// The fill in the outer clip's corner, which the inner clip
		// does not reach.
		at := image.Rect(10, 10, 30, 30)
		hit := false
		for _, r := range damage {
			hit = hit || at.In(r)
		}
		if !hit {
			t.Errorf("%d to %d ops: damage %v misses %v", len(frames[0].Ops), len(frames[1].Ops), damage, at)
		}
	}
	nested[0].Transform = Translation(100.5, 100.25)
	with.Ops = append(append([]Op{outer, corner}, nested...), Op{Kind: OpPopClip})
	d := Differ{Reach: 2}
	d.Diff(without)
	d.Remember(without)
	damage, _ := d.Diff(with)
	for _, r := range damage {
		if r.Overlaps(image.Rect(10, 10, 30, 30)) {
			t.Errorf("a clip only moved coming: damage %v reaches the outer clip's corner", damage)
		}
	}
}

// TestDamageOfClipInRoundedCorners checks that a clip reaching into the
// corners of a rounded one changes all the rounded one cuts as it comes
// and goes, which the GPU's renderers then cut with a mask (#1418), and
// that one away from them changes only what it cuts.
func TestDamageOfClipInRoundedCorners(t *testing.T) {
	outer := Op{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 150, H: 150}, Radii: Uniform(8)}
	corner := Op{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 20, H: 20}, Color: Color{R: 255, A: 255}}
	nested := func(r Rect) []Op {
		return []Op{{Kind: OpPushClip, Rect: r}, {Kind: OpFill, Rect: r, Color: Color{G: 255, A: 255}}, {Kind: OpPopClip}}
	}
	without := &Scene{Width: 200, Height: 200, Ops: []Op{outer, corner, {Kind: OpPopClip}}}
	at := image.Rect(10, 10, 30, 30)
	for _, c := range []struct {
		r       Rect
		corners bool
	}{{Rect{X: 10, Y: 120, W: 150, H: 40}, true}, {Rect{X: 60, Y: 60, W: 40, H: 40}, false}} {
		with := &Scene{Width: 200, Height: 200, Ops: append(append([]Op{outer, corner}, nested(c.r)...), Op{Kind: OpPopClip})}
		for _, frames := range [][2]*Scene{{without, with}, {with, without}} {
			d := Differ{Reach: 2}
			d.Diff(frames[0])
			d.Remember(frames[0])
			damage, ok := d.Diff(frames[1])
			if !ok {
				t.Fatal("no damage found")
			}
			hit := false
			for _, r := range damage {
				hit = hit || r.Overlaps(at)
			}
			if hit != c.corners {
				t.Errorf("clip %v, %d to %d ops: damage %v reaches %v: %v", c.r, len(frames[0].Ops), len(frames[1].Ops), damage, at, hit)
			}
		}
	}
}

// A masked clip changes all it cuts when its mask changes: the stops of
// its gradient, or the pixels of its image; and nothing when they are the
// same, in another frame's scene.
func TestDamageOfMaskedClip(t *testing.T) {
	img := NewImageRGBA(4, 4, make([]byte, 64))
	build := func(alpha uint8) *Scene {
		return &Scene{Width: 100, Height: 100, Stops: []Stop{{Color: Color{A: alpha}}, {Color: Color{}, At: 1}}, Ops: []Op{
			{Kind: OpPushClip, Rect: Rect{X: 10, Y: 10, W: 40, H: 40}, Paint: PaintLinearStops, Gradient: [4]float32{10, 10, 50, 10}, Stops: [2]int32{0, 2}},
			{Kind: OpFill, Rect: Rect{X: 10, Y: 10, W: 40, H: 40}, Color: Color{R: 255, A: 255}},
			{Kind: OpPopClip},
			{Kind: OpPushClip, Rect: Rect{X: 60, Y: 60, W: 4, H: 4}, Image: img, Src: Rect{W: 4, H: 4}},
			{Kind: OpFill, Rect: Rect{X: 60, Y: 60, W: 4, H: 4}, Color: Color{G: 255, A: 255}},
			{Kind: OpPopClip},
		}}
	}
	d := Differ{Reach: 1}
	d.Diff(build(255))
	d.Remember(build(255))
	if damage, ok := d.Diff(build(255)); !ok || len(damage) != 0 {
		t.Fatalf("the same masks: damage %v, %v", damage, ok)
	}
	d.Remember(build(255))
	if damage, ok := d.Diff(build(128)); !ok || len(damage) != 1 || !image.Rect(10, 10, 50, 50).In(damage[0]) {
		t.Errorf("other stops: damage %v, %v", damage, ok)
	}
	d.Remember(build(128))
	img.Changed()
	if damage, ok := d.Diff(build(128)); !ok || len(damage) != 1 || !image.Rect(60, 60, 64, 64).In(damage[0]) {
		t.Errorf("the image changed: damage %v, %v", damage, ok)
	}
}

// An effect painting quads (EffectOp.Instances) damages where it paints
// when a quad's data changes, though the scene reuses its words next
// frame (Words), and nothing when the quads stay the same.
func TestDamageOfInstances(t *testing.T) {
	fx := &Effect{Name: "quads"}
	words := make([]uint32, 2*(InstanceHead+1))
	build := func(data float32) *Scene {
		for i, q := range []Rect{{X: 10, Y: 10, W: 20, H: 20}, {X: 50, Y: 40, W: 20, H: 20}} {
			w := words[i*(InstanceHead+1):]
			for k, v := range []float32{q.X, q.Y, q.W, q.H, q.W, q.H, data} {
				w[k] = math.Float32bits(v)
			}
		}
		return &Scene{Width: 100, Height: 100, Words: words,
			Ops:     []Op{{Kind: OpEffect, Rect: Rect{X: 10, Y: 10, W: 60, H: 50}}},
			Effects: []EffectOp{{Effect: fx, Instances: words, InstanceWords: InstanceHead + 1}}}
	}
	d := Differ{Reach: 1}
	d.Diff(build(1))
	d.Remember(build(1))
	if damage, ok := d.Diff(build(1)); !ok || len(damage) != 0 {
		t.Fatalf("the same quads: damage %v, %v", damage, ok)
	}
	d.Remember(build(1))
	if damage, ok := d.Diff(build(2)); !ok || len(damage) != 1 || !image.Rect(10, 10, 70, 60).In(damage[0]) {
		t.Errorf("other data: damage %v, %v", damage, ok)
	}
}

func TestDamageOfEffectImages(t *testing.T) {
	fx := &Effect{Name: "image-fx"}
	imgA := NewImageRGBA(16, 16, make([]byte, 16*16*4))
	imgB := NewImageRGBA(16, 16, make([]byte, 16*16*4))
	build := func(img *Image) *Scene {
		return &Scene{
			Width:   120,
			Height:  120,
			Ops:     []Op{{Kind: OpEffect, Rect: Rect{X: 20, Y: 30, W: 40, H: 50}}},
			Effects: []EffectOp{{Effect: fx, Images: [4]*Image{img}}},
		}
	}

	d := Differ{Reach: 1}
	d.Diff(build(imgA))
	d.Remember(build(imgA))
	if damage, ok := d.Diff(build(imgA)); !ok || len(damage) != 0 {
		t.Fatalf("unchanged effect image caused damage %v, ok=%v", damage, ok)
	}

	d.Remember(build(imgA))
	imgA.Changed()
	if damage, ok := d.Diff(build(imgA)); !ok || len(damage) != 1 || !image.Rect(20, 30, 60, 80).In(damage[0]) {
		t.Fatalf("mutated effect image damage %v, ok=%v", damage, ok)
	}

	d.Remember(build(imgA))
	if damage, ok := d.Diff(build(imgB)); !ok || len(damage) != 1 || !image.Rect(20, 30, 60, 80).In(damage[0]) {
		t.Fatalf("swapped effect image damage %v, ok=%v", damage, ok)
	}
}

func BenchmarkDiffEffectWithImage(b *testing.B) {
	img := NewImageRGBA(64, 64, make([]byte, 64*64*4))
	fx := &Effect{Name: "tinted-image"}
	ops := make([]Op, 0, 64)
	for i := range 60 {
		ops = append(ops, Op{Kind: OpFill, Rect: Rect{X: float32(i * 10), Y: 20, W: 8, H: 8}, Color: Color{R: 120, G: 140, B: 200, A: 255}})
	}
	ops = append(ops, Op{Kind: OpEffect, Rect: Rect{X: 100, Y: 100, W: 128, H: 128}, Start: 0, End: 1})
	s := &Scene{
		Width:   1280,
		Height:  800,
		Ops:     ops,
		Effects: []EffectOp{{Effect: fx, Images: [4]*Image{img}}},
	}
	d := Differ{Reach: 1}
	d.Diff(s)
	d.Remember(s)
	totalDamage := 0
	for b.Loop() {
		dmg, _ := d.Diff(s)
		totalDamage += len(dmg)
		d.Remember(s)
	}
	b.ReportMetric(float64(totalDamage)/float64(b.N), "damage-rects")
}

func TestPartialDiffMatchesFull(t *testing.T) {
	maskAtlas := NewAtlas(1, 256, 256)
	mx, my, ok := maskAtlas.Alloc(8, 12)
	if !ok {
		t.Fatal("atlas alloc failed")
	}
	img := NewImageRGBA(16, 16, make([]byte, 16*16*4))
	fxImg := NewImageRGBA(8, 8, make([]byte, 8*8*4))
	fx := &Effect{Name: "fx-with-img"}

	makeDocScene := func(id uint64, partial PartEdit, caretAlpha uint8, caretCount int, imgInPrefix bool, reshapedInPart bool) *Scene {
		s := &Scene{
			Width: 1200, Height: 800,
			MaskAtlas: maskAtlas,
			ID:        id,
			Partial:   partial,
			Stops:     []Stop{{Color: Color{A: 255}, At: 0}, {Color: Color{A: 0}, At: 1}},
			Effects:   []EffectOp{{Effect: fx, Images: [4]*Image{fxImg}}},
		}
		// Prefix: background, clipped multi-line text, optional prefix image/effect.
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{W: 1200, H: 800}, Color: Color{R: 255, G: 255, B: 255, A: 255}})
		if imgInPrefix {
			s.Ops = append(s.Ops, Op{Kind: OpImage, Rect: Rect{X: 20, Y: 20, W: 16, H: 16}, Image: img, Src: Rect{W: 16, H: 16}})
			s.Ops = append(s.Ops, Op{Kind: OpEffect, Rect: Rect{X: 50, Y: 20, W: 8, H: 8}, Start: 0, End: 1})
		}
		s.Ops = append(s.Ops, Op{Kind: OpPushClip, Rect: Rect{X: 80, Y: 40, W: 1000, H: 700}})
		for row := range 30 {
			start := int32(len(s.Glyphs))
			for col := range 40 {
				s.Glyphs = append(s.Glyphs, Glyph{
					X: float32(90 + col*10), Y: float32(60 + row*22),
					W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
					Color: Color{R: 20, G: 20, B: 20, A: 255},
				})
			}
			s.Ops = append(s.Ops, Op{Kind: OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
		}
		opStart := len(s.Ops)
		for k := range caretCount {
			op := Op{Kind: OpFill, Rect: Rect{X: float32(200 + k*6), Y: 200, W: 2, H: 18}, Color: Color{R: 30, G: 110, B: 240, A: caretAlpha}}
			if reshapedInPart {
				op.Transform = Rotation(15)
			}
			s.Ops = append(s.Ops, op)
		}
		if s.Partial.BaseID != 0 {
			s.Partial.OpStart = opStart
		}
		s.Ops = append(s.Ops, Op{Kind: OpPopClip})
		// Suffix edge fades.
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{X: 80, Y: 40, W: 1000, H: 24}, Paint: PaintLinearStops, Gradient: [4]float32{80, 40, 80, 64}, Stops: [2]int32{0, 2}})
		return s
	}

	cloneForFull := func(s *Scene) *Scene {
		c := *s
		c.Partial = PartEdit{}
		return &c
	}

	assertDiffMatches := func(name string, df, dr *Differ, s *Scene, wantFast bool) {
		t.Helper()
		fullS := cloneForFull(s)
		dmgFast, okFast := df.Diff(s)
		if df.fast != wantFast {
			t.Fatalf("%s: fast=%v, want %v", name, df.fast, wantFast)
		}
		dmgRef, okRef := dr.Diff(fullS)
		if okFast != okRef || !slices.Equal(dmgFast, dmgRef) {
			t.Fatalf("%s: damage mismatch fast=(%v, %v) vs ref=(%v, %v)", name, dmgFast, okFast, dmgRef, okRef)
		}
		for j := range s.Ops {
			if got, want := df.OpBound(j), dr.OpBound(j); got != want {
				t.Fatalf("%s: OpBound(%d) = %v, want %v", name, j, got, want)
			}
		}
		if got, want := df.Bounds(), dr.Bounds(); !slices.Equal(got, want) {
			t.Fatalf("%s: Bounds() len %d != %d", name, len(got), len(want))
		}
		df.Remember(s)
		dr.Remember(fullS)
		for j := range s.Ops {
			if got, want := df.RememberedBound(j), dr.RememberedBound(j); got != want {
				t.Fatalf("%s: RememberedBound(%d) = %v, want %v", name, j, got, want)
			}
		}
	}

	df := Differ{Reach: 2}
	dr := Differ{Reach: 2}
	const caretBase = 34
	s0 := makeDocScene(10, PartEdit{}, 255, 1, true, false)
	markG := len(s0.Glyphs)
	assertDiffMatches("init", &df, &dr, s0, false)

	// 1. Equal-length alpha update in partial edit.
	s1 := makeDocScene(11, PartEdit{BaseID: 10, OldOpEnd: caretBase + 1, NewOpEnd: caretBase + 1, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 128, 1, true, false)
	assertDiffMatches("alpha-change", &df, &dr, s1, true)

	// 2. Unchanged frame via ReuseUnchanged keeps subsequent fast path alive.
	s2 := makeDocScene(12, PartEdit{BaseID: 11, OldOpEnd: caretBase + 1, NewOpEnd: caretBase + 1, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 128, 1, true, false)
	dmgUnchanged, ok := df.Diff(s2)
	if !ok || len(dmgUnchanged) != 0 || !df.fast {
		t.Fatalf("unchanged diff = (%v, %v, fast=%v)", dmgUnchanged, ok, df.fast)
	}
	df.ReuseUnchanged(s2)
	dr.Diff(cloneForFull(s2))
	dr.Remember(cloneForFull(s2))

	// 3. Growing splice (1 -> 3 ops).
	s3 := makeDocScene(13, PartEdit{BaseID: 12, OldOpEnd: caretBase + 1, NewOpEnd: caretBase + 3, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 90, 3, true, false)
	assertDiffMatches("grow", &df, &dr, s3, true)

	// 4. Shrinking splice (3 -> 2 ops) + second consecutive partial edit.
	s4 := makeDocScene(14, PartEdit{BaseID: 13, OldOpEnd: caretBase + 3, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 200, 2, true, false)
	assertDiffMatches("shrink", &df, &dr, s4, true)

	// 5. Fallback on BaseID mismatch.
	s5 := makeDocScene(15, PartEdit{BaseID: 999, OldOpEnd: caretBase + 2, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 180, 2, true, false)
	assertDiffMatches("base-mismatch", &df, &dr, s5, false)

	// 6. Fallback on atlas change.
	ax, ay, ok := maskAtlas.Alloc(8, 8)
	if !ok {
		t.Fatal("second atlas alloc failed")
	}
	maskAtlas.Put(ax, ay, 8, 8, make([]byte, 64), 8)
	s6 := makeDocScene(16, PartEdit{BaseID: 15, OldOpEnd: caretBase + 2, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 140, 2, true, false)
	assertDiffMatches("atlas-change", &df, &dr, s6, false)

	// 7. Fallback on prefix OpImage version change.
	img.Changed()
	s7 := makeDocScene(17, PartEdit{BaseID: 16, OldOpEnd: caretBase + 2, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 100, 2, true, false)
	assertDiffMatches("img-change", &df, &dr, s7, false)

	// 8. Fallback on prefix EffectOp.Images version change.
	fxImg.Changed()
	s8 := makeDocScene(18, PartEdit{BaseID: 17, OldOpEnd: caretBase + 2, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 70, 2, true, false)
	assertDiffMatches("fx-img-change", &df, &dr, s8, false)

	// 9. Fallback when repainted part contains a Reshaped transform.
	s9 := makeDocScene(19, PartEdit{BaseID: 18, OldOpEnd: caretBase + 2, NewOpEnd: caretBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 50, 2, true, true)
	assertDiffMatches("reshaped-part", &df, &dr, s9, false)
}

func BenchmarkDifferPartialCaret(b *testing.B) {
	atlas := NewAtlas(1, 512, 512)
	mx, my, _ := atlas.Alloc(8, 12)
	makeScene := func(id uint64, baseID uint64) (*Scene, PartEdit) {
		s := &Scene{
			Width: 2560, Height: 1600, MaskAtlas: atlas, ID: id,
			Stops: []Stop{{Color: Color{A: 255}, At: 0}, {Color: Color{A: 0}, At: 1}},
		}
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{W: 2560, H: 1600}, Color: Color{R: 250, G: 250, B: 250, A: 255}})
		s.Ops = append(s.Ops, Op{Kind: OpPushClip, Rect: Rect{X: 200, Y: 80, W: 1600, H: 1440}})
		for row := range 80 {
			start := int32(len(s.Glyphs))
			for col := range 60 {
				s.Glyphs = append(s.Glyphs, Glyph{
					X: float32(220 + col*11), Y: float32(100 + row*18),
					W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
					Color: Color{R: 30, G: 30, B: 30, A: 255},
				})
			}
			s.Ops = append(s.Ops, Op{Kind: OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
		}
		opStart := len(s.Ops)
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{X: 300, Y: 300, W: 3, H: 22}, Color: Color{R: 20, G: 100, B: 240, A: 255}})
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{X: 200, Y: 80, W: 1600, H: 32}, Paint: PaintLinearStops, Gradient: [4]float32{200, 80, 200, 112}, Stops: [2]int32{0, 2}})
		s.Ops = append(s.Ops, Op{Kind: OpFill, Rect: Rect{X: 200, Y: 1488, W: 1600, H: 32}, Paint: PaintLinearStops, Gradient: [4]float32{200, 1488, 200, 1520}, Stops: [2]int32{0, 2}})
		opEnd := len(s.Ops)
		s.Ops = append(s.Ops, Op{Kind: OpPopClip})
		edit := PartEdit{
			BaseID: baseID, OpStart: opStart, OldOpEnd: opEnd, NewOpEnd: opEnd,
			GlyphMark: len(s.Glyphs), StopMark: len(s.Stops),
		}
		if baseID != 0 {
			s.Partial = edit
		}
		return s, edit
	}
	for _, partial := range []bool{false, true} {
		name := "full"
		if partial {
			name = "partial"
		}
		b.Run(name, func(b *testing.B) {
			s, edit := makeScene(1, 0)
			d := Differ{Reach: 2}
			d.Diff(s)
			d.Remember(s)
			caretIdx := len(s.Ops) - 4
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				s.Ops[caretIdx].Color.A = byte(1 + i%250)
				if partial {
					edit.BaseID = s.ID
					s.ID = uint64(i + 2)
					s.Partial = edit
				} else {
					s.ID = uint64(i + 2)
					s.Partial = PartEdit{}
				}
				d.Diff(s)
				d.Remember(s)
			}
		})
	}
}
