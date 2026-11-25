package gpu

import (
	"fmt"
	"image"
	"reflect"
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

func TestAges(t *testing.T) {
	var forgot []uintptr
	a := Ages{Forget: func(id uintptr) { forgot = append(forgot, id) }}
	caret := image.Rect(10, 10, 12, 30)
	clock := image.Rect(100, 0, 140, 20)
	both := fmt.Sprint([]image.Rectangle{caret, clock})
	// Three buffers in turn, each new: drawn whole.
	for id := uintptr(1); id <= 3; id++ {
		a.Add([]image.Rectangle{caret}, false)
		if _, known := a.Take(id); known {
			t.Fatalf("buffer %d, new, is known", id)
		}
	}
	// Buffer 1 lacks the two frames drawn since it was: the caret's, apart
	// from the clock's.
	a.Add([]image.Rectangle{clock}, false)
	if got, known := a.Take(1); !known || fmt.Sprint(got) != both {
		t.Errorf("buffer 1 lacks %v (known %v), want %v", got, known, both)
	}
	// Buffer 2 lacks the frame drawn into 3, the clock's and nothing.
	a.Add(nil, false)
	if got, known := a.Take(2); !known || fmt.Sprint(got) != both {
		t.Errorf("buffer 2 lacks %v (known %v), want %v", got, known, both)
	}
	// A frame changing everything makes every buffer drawn whole.
	a.Add(nil, true)
	if _, known := a.Take(3); known {
		t.Error("buffer 3 is known after a frame changing everything")
	}
	a.Add([]image.Rectangle{caret}, false)
	if _, known := a.Take(1); known {
		t.Error("buffer 1 is known after a frame changing everything")
	}
	// A fifth buffer takes the place of the one drawn least recently.
	a.Take(4)
	a.Take(5)
	if len(forgot) != 1 || forgot[0] != 2 {
		t.Errorf("forgot %v, want [2]", forgot)
	}
	a.Reset()
	if len(forgot) != 5 || a.Has(1) {
		t.Errorf("after Reset, forgot %v and tracks buffer 1: %v", forgot, a.Has(1))
	}
}

func TestAddRect(t *testing.T) {
	var rects []image.Rectangle
	// Rectangles apart stay apart; one meeting two takes them in.
	rects = addRect(rects, image.Rect(0, 0, 10, 10))
	rects = addRect(rects, image.Rect(50, 0, 60, 10))
	if len(rects) != 2 {
		t.Fatalf("two apart: %v", rects)
	}
	rects = addRect(rects, image.Rect(5, 5, 55, 8))
	if fmt.Sprint(rects) != fmt.Sprint([]image.Rectangle{image.Rect(0, 0, 60, 10)}) {
		t.Errorf("one meeting both: %v", rects)
	}
	// Past maxRects, the two nearest become one, and none overlap.
	rects = rects[:0]
	for i := range maxRects + 1 {
		rects = addRect(rects, image.Rect(i*100, 0, i*100+10, 10))
	}
	rects = addRect(rects, image.Rect(1000, 0, 1010, 10)) // nearest the last
	if len(rects) != maxRects {
		t.Fatalf("%d rectangles, want %d: %v", len(rects), maxRects, rects)
	}
	for i, r := range rects {
		for _, o := range rects[i+1:] {
			if r.Overlaps(o) {
				t.Errorf("%v overlaps %v", r, o)
			}
		}
	}
}

func TestPartial(t *testing.T) {
	var p Part
	// Within the frame.
	if !p.Set([]image.Rectangle{image.Rect(-5, 10, 20, 30)}, 100, 100, nil) || fmt.Sprint(p.Rects) != "[{0 10 20 30}]" {
		t.Errorf("Set = %v", p.Rects)
	}
	// Two corners stay apart.
	if !p.Set([]image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(90, 90, 100, 100)}, 100, 100, nil) || fmt.Sprint(p.Rects) != "[{0 0 10 10} {90 90 100 100}]" {
		t.Errorf("two corners: %v", p.Rects)
	}
	// Half the frame or more, together, is drawn whole.
	if p.Set([]image.Rectangle{image.Rect(0, 0, 100, 30), image.Rect(0, 60, 100, 80)}, 100, 100, nil) {
		t.Error("half the frame drawn in part")
	}
	// Nothing changed in the frame: nothing to draw.
	if !p.Set([]image.Rectangle{image.Rect(200, 0, 210, 10)}, 100, 100, nil) || len(p.Rects) != 0 {
		t.Errorf("outside: %v", p.Rects)
	}
	// Grown until what it grows by is in it: a backdrop from 40 to 60
	// meets the area, an effect reading one from 0 to 45 then meets that.
	grow := func(d []image.Rectangle) []image.Rectangle {
		for _, b := range []image.Rectangle{image.Rect(40, 0, 60, 10), image.Rect(0, 0, 45, 10)} {
			for _, r := range d {
				if r.Overlaps(b) {
					d = append(d, b)
					break
				}
			}
		}
		return d
	}
	if !p.Set([]image.Rectangle{image.Rect(50, 0, 55, 5), image.Rect(80, 80, 90, 90)}, 100, 100, grow) || fmt.Sprint(p.Rects) != "[{80 80 90 90} {0 0 60 10}]" {
		t.Errorf("grown: %v, want [{80 80 90 90} {0 0 60 10}]", p.Rects)
	}
}

func TestIncrementalBuildMatchesFull(t *testing.T) {
	atlas := scene.NewAtlas(1, 512, 512)
	mx, my, ok := atlas.Alloc(8, 12)
	if !ok {
		t.Fatal("atlas alloc failed")
	}
	atlas.Put(mx, my, 8, 12, make([]byte, 96), 8)
	img := scene.NewImageRGBA(32, 32, make([]byte, 32*32*4))
	fx := &scene.Effect{Name: "chrome"}

	makeScene := func(id uint64, partial scene.PartEdit, caretAlpha uint8, caretCount int) *scene.Scene {
		s := &scene.Scene{
			Width: 1600, Height: 1000, MaskAtlas: atlas, ID: id, Partial: partial,
			Stops: []scene.Stop{{Color: scene.Color{A: 255}, At: 0}, {Color: scene.Color{A: 0}, At: 1}},
			Effects: []scene.EffectOp{{Effect: fx}},
		}
		// Large opaque background (with OpaqueCores & culling candidate).
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 1600, H: 1000}, Color: scene.Color{R: 250, G: 250, B: 250, A: 255}})
		// Prefix image.
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 20, Y: 20, W: 32, H: 32}, Image: img, Src: scene.Rect{W: 32, H: 32}})
		//Prefix shadow card (exercises bands/joinLast boundary).
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpShadow, Rect: scene.Rect{X: 80, Y: 40, W: 900, H: 800}, Radii: scene.Uniform(8), Cast: scene.Rect{X: 90, Y: 50, W: 880, H: 780}, CastRadii: scene.Uniform(8), Blur: 8, Color: scene.Color{A: 40}})
		// Translated fill to exercise FoldActive + clip.
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 100, Y: 60, W: 860, H: 760}})
		for row := range 30 {
			start := int32(len(s.Glyphs))
			for col := range 40 {
				s.Glyphs = append(s.Glyphs, scene.Glyph{
					X: float32(110 + col*11), Y: float32(80 + row*20),
					W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
					Color: scene.Color{R: 30, G: 30, B: 30, A: 255},
				})
			}
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
		}
		opStart := len(s.Ops)
		for k := range caretCount {
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: float32(250 + k*6), Y: 200, W: 3, H: 20}, Radii: scene.Uniform(1), Color: scene.Color{R: 20, G: 100, B: 240, A: caretAlpha}})
		}
		// Gradient edge fade in part.
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 100, Y: 60, W: 860, H: 32}, Paint: scene.PaintLinearStops, Gradient: [4]float32{100, 60, 100, 92}, Stops: [2]int32{0, 2}})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
		// Suffix badge & effect outside clipped scroll area.
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 1200, Y: 40, W: 80, H: 24}, Radii: scene.Uniform(6), Color: scene.Color{R: 60, G: 120, B: 200, A: 255}})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpEffect, Rect: scene.Rect{X: 1200, Y: 80, W: 40, H: 40}})
		if partial.BaseID != 0 {
			s.Partial.OpStart = opStart
		}
		return s
	}

	assertMatch := func(name string, inc, ref *Builder, s *scene.Scene, wantInc bool) {
		t.Helper()
		fullS := *s
		fullS.Partial = scene.PartEdit{}
		imgFn := func(im *scene.Image) uintptr { return uintptr(im.ID()) }
		inc.Build(s, imgFn)
		if gotInc := inc.incCutOp > 0; gotInc != wantInc {
			t.Fatalf("%s: incremental=%v (cutOp=%d, cutBatch=%d), want %v", name, gotInc, inc.incCutOp, inc.incCutBatch, wantInc)
		}
		ref.Build(&fullS, imgFn)
		if !slices.Equal(inc.Instances, ref.Instances) {
			t.Fatalf("%s: Instances mismatch (%d vs %d)", name, len(inc.Instances), len(ref.Instances))
		}
		if !reflect.DeepEqual(inc.Batches, ref.Batches) {
			t.Fatalf("%s: Batches mismatch (%+v vs %+v)", name, inc.Batches, ref.Batches)
		}
		if !slices.Equal(inc.Stops, ref.Stops) {
			t.Fatalf("%s: Stops mismatch", name)
		}
		if !slices.Equal(inc.Backdrops, ref.Backdrops) {
			t.Fatalf("%s: Backdrops mismatch", name)
		}
		if !slices.Equal(inc.OpStart, ref.OpStart) {
			t.Fatalf("%s: OpStart mismatch (%v vs %v)", name, inc.OpStart, ref.OpStart)
		}
		if inc.Culled != ref.Culled || inc.OpaqueCores != ref.OpaqueCores {
			t.Fatalf("%s: Culled/OpaqueCores = (%d,%d) vs (%d,%d)", name, inc.Culled, inc.OpaqueCores, ref.Culled, ref.OpaqueCores)
		}
	}

	const opBase = 34
	inc := Builder{Classes: true, ClipSkip: true, Opaque: true}
	ref := Builder{Classes: true, ClipSkip: true, Opaque: true}
	s0 := makeScene(1, scene.PartEdit{}, 255, 1)
	markG := len(s0.Glyphs)
	assertMatch("init", &inc, &ref, s0, false)

	s1 := makeScene(2, scene.PartEdit{BaseID: 1, OldOpEnd: opBase + 2, NewOpEnd: opBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 128, 1)
	assertMatch("alpha-change", &inc, &ref, s1, true)

	s2 := makeScene(3, scene.PartEdit{BaseID: 2, OldOpEnd: opBase + 2, NewOpEnd: opBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 0, 0)
	// Keep 1 edge gradient (opBase + 1):
	s2 = makeScene(3, scene.PartEdit{BaseID: 2, OldOpEnd: opBase + 2, NewOpEnd: opBase + 1, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 0, 0)
	assertMatch("shrink-to-0-caret", &inc, &ref, s2, true)

	s3 := makeScene(4, scene.PartEdit{BaseID: 3, OldOpEnd: opBase + 1, NewOpEnd: opBase + 4, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 200, 3)
	assertMatch("grow-to-3-carets", &inc, &ref, s3, true)

	s4 := makeScene(5, scene.PartEdit{BaseID: 4, OldOpEnd: opBase + 4, NewOpEnd: opBase + 2, GlyphMark: markG, StopMark: 2, EffectMark: 1}, 64, 1)
	assertMatch("shrink-to-1-caret", &inc, &ref, s4, true)
}

func TestPartCompactMatchesWhole(t *testing.T) {
	atlas := scene.NewAtlas(1, 512, 512)
	mx, my, _ := atlas.Alloc(8, 12)
	s := &scene.Scene{
		Width: 2560, Height: 1600, MaskAtlas: atlas,
		Stops: []scene.Stop{{Color: scene.Color{A: 255}, At: 0}, {Color: scene.Color{A: 0}, At: 1}},
	}
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 2560, H: 1600}, Color: scene.Color{R: 250, G: 250, B: 250, A: 255}})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 100, Y: 50, W: 1400, H: 1400}})
	for row := range 60 {
		start := int32(len(s.Glyphs))
		for col := range 50 {
			s.Glyphs = append(s.Glyphs, scene.Glyph{
				X: float32(120 + col*11), Y: float32(80 + row*20),
				W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
				Color: scene.Color{R: 20, G: 20, B: 20, A: 255},
			})
		}
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
	}
	caretIdx := len(s.Ops)
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 250, Y: 200, W: 2, H: 18}, Color: scene.Color{R: 10, G: 100, B: 255, A: 200}})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})

	var d scene.Differ
	d.Reach = 2
	d.Diff(s)
	d.Remember(s)
	s.Ops[caretIdx].Color.A = 120
	dmg, ok := d.Diff(s)
	if !ok {
		t.Fatal("diff failed")
	}

	bld := Builder{Classes: true, ClipSkip: true, Opaque: true}
	bld.Build(s, nil)
	var p Part
	if !p.Set(dmg, s.Width, s.Height, nil) {
		t.Fatal("expected partial draw")
	}
	p.Compact(&bld, d.OpBound)
	if len(p.Instances) >= len(bld.Instances)/20 {
		t.Fatalf("Part.Compact kept %d / %d instances; expected >95%% reduction", len(p.Instances), len(bld.Instances))
	}
	if len(p.Batches) == 0 {
		t.Fatal("Part.Compact kept 0 batches")
	}
}
