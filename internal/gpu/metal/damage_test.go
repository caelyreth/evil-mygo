//go:build darwin && !ios

package metal

import (
	"errors"
	"fmt"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// TestPartialFramesMatchWhole draws random scenes changing a little at a
// time into three targets in turn, as a CAMetalLayer's drawables are,
// each frame only where it changed since its target was drawn, and
// requires the bytes of each frame drawn whole: in BGRA8 and float16
// targets, over an opaque color and a translucent one.
func TestPartialFramesMatchWhole(t *testing.T) {
	seeds := uint64(60)
	if testing.Short() {
		seeds = 10
	}
	for _, wide := range []bool{false, true} {
		frames, partial := 0, 0
		for seed := range seeds {
			// A renderer of their own, as each has an atlas of its own.
			r, err := newRenderer()
			if err != nil {
				unavailable(t, err)
			}
			m := gputest.NewChanging(seed)
			if seed%3 == 2 {
				m.Clear = scene.Color{R: 20, G: 40, B: 200, A: 77}
			}
			s := m.Scene()
			t.Run(fmt.Sprintf("wide=%v/seed=%d", wide, seed), func(t *testing.T) {
				partial += partialFrames(t, r, s, wide, 30, func(int) string { return m.Change(s) })
				frames += 30
			})
			r.Release()
		}
		// Many frames change a little, though much of so small a scene:
		// those after the first three, one for each target, are drawn in
		// part but where they change half of it.
		if partial < frames/3 {
			t.Errorf("wide %v: %d frames of %d drawn in part", wide, partial, frames)
		}
	}
}

// partialFrames draws frames of s, changed by change before each but the
// first, into three targets in turn, each only where it changed, and
// requires the bytes of each frame drawn whole; it returns how many were
// drawn in part.
func partialFrames(t *testing.T, r *Renderer, s *scene.Scene, wide bool, frames int, change func(step int) string) (partial int) {
	t.Helper()
	format, bpp := uint(pixelFormatBGRA8Unorm), 4
	r.cur, r.wide = &r.formats[0], wide
	if wide {
		if !r.SetWide(true) {
			t.Skip("no float16 drawables")
		}
		r.cur, format, bpp = &r.formats[1], pixelFormatRGBA16Float, 8
	}
	cur := r.cur
	r.forgetFrames()
	defer r.forgetFrames()
	var targets [3]id
	pool(func() {
		for i := range targets {
			targets[i] = r.newReadbackTexture(s.Width, s.Height, format, usageRenderTarget|usageShaderRead)
		}
	})
	defer pool(func() {
		for i := range targets {
			release(&targets[i])
		}
	})
	read := func(tex id) (raw []byte, err error) {
		cb := send(r.queue, "commandBuffer")
		blit := send(cb, "blitCommandEncoder")
		send(blit, "synchronizeResource:", tex)
		send(blit, "endEncoding")
		send(cb, "commit")
		send(cb, "waitUntilCompleted")
		raw = make([]byte, s.Width*s.Height*bpp)
		msgGetBytes(tex, sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&raw[0]), uint(s.Width*bpp),
			mtlRegion{W: uint(s.Width), H: uint(s.Height), D: 1}, 0)
		return raw, nil
	}
	for step := range frames {
		what := "first"
		if step > 0 {
			what = change(step)
		}
		var got []byte
		var err error
		var part string
		pool(func() {
			r.waitLast()
			r.cur = cur
			tex := targets[step%len(targets)]
			r.diff.Wide = wide
			damage, known := r.diff.Diff(s)
			cb, e := r.drawFrame(s, damage, known, func() id { return tex })
			if e != nil {
				err = e
				return
			}
			part = "whole"
			if r.redrew == redrewPart {
				partial++
				part = r.part.String()
			}
			send(cb, "commit")
			send(cb, "waitUntilCompleted")
			if int(send(cb, "status")) == statusError {
				err = errors.New("the frame failed")
				return
			}
			got, err = read(tex)
		})
		if err != nil {
			t.Fatal(err)
		}
		want := renderRaw(t, r, s, wide)
		if n := differing(got, want); n != 0 {
			var at []string
			for i := range got {
				if got[i] != want[i] && len(at) < 6 {
					px := i / bpp
					at = append(at, fmt.Sprintf("(%d,%d)c%d %d≠%d", px%s.Width, px/s.Width, i%bpp, got[i], want[i]))
				}
			}
			t.Fatalf("step %d (%s), redrew %s: %d bytes differ from the frame drawn whole, at %v", step, what, part, n, at)
		}
	}
	return partial
}

// TestPartialFramesOfOneChange checks that a caret blinking redraws its own
// box alone once each target was drawn whole.
func TestPartialFramesOfOneChange(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	m := gputest.NewChanging(5)
	s := m.Scene()
	caret := scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 100, Y: 100, W: 2, H: 18}, Color: scene.Color{A: 255}}
	s.Ops = append(s.Ops, caret)
	var areas []string
	redrew := func() string {
		if r.redrew != redrewPart {
			return "whole"
		}
		a := r.part.Bounds()
		return fmt.Sprintf("%d×%d", a.Right-a.Left, a.Bottom-a.Top)
	}
	partialFrames(t, r, s, false, 8, func(step int) string {
		areas = append(areas, redrew())
		if step%2 == 1 {
			s.Ops[len(s.Ops)-1].Color.A = 0
		} else {
			s.Ops[len(s.Ops)-1].Color.A = 255
		}
		return fmt.Sprint("caret ", step%2)
	})
	areas = append(areas, redrew())
	// The three targets drawn whole, then each the caret's box, grown by
	// the reach of the GPU's quads.
	want := []string{"whole", "whole", "whole", "6×22", "6×22", "6×22", "6×22", "6×22"}
	if fmt.Sprint(areas) != fmt.Sprint(want) {
		t.Errorf("redrew %v, want %v", areas, want)
	}
}

// TestPartialFramesApart checks that two changes in opposite corners, a
// spinner's and a progress bar's, redraw two rectangles once each target
// was drawn whole, not all between, with the bytes of the frame drawn
// whole.
func TestPartialFramesApart(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	s := gputest.NewChanging(5).Scene()
	s.Ops = append(s.Ops,
		scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 4, Y: 4, W: 12, H: 12}, Radii: scene.Uniform(6), Color: scene.Color{R: 200, A: 255}},
		scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: float32(s.Width) - 70, Y: float32(s.Height) - 14, W: 60, H: 6}, Radii: scene.Uniform(3), Color: scene.Color{B: 200, A: 255}})
	var rects []int
	partialFrames(t, r, s, false, 7, func(step int) string {
		if r.redrew == redrewPart {
			rects = append(rects, len(r.part.Rects))
		}
		n := len(s.Ops)
		s.Ops[n-2].Color.G += 30
		s.Ops[n-1].Rect.W -= 5
		return fmt.Sprint("spinner and progress ", step)
	})
	if r.redrew == redrewPart {
		rects = append(rects, len(r.part.Rects))
	}
	// The three targets drawn whole, then each two rectangles.
	if fmt.Sprint(rects) != "[2 2 2 2]" {
		t.Errorf("redrew rectangles %v, want [2 2 2 2]", rects)
	}
}

// TestPartialFramesIncrementalCaret checks that triple-buffered partial redraws
// with Scene.Partial, incremental GPU Builder, and Part.Compact match full-frame
// rendering byte-for-byte over >240 frames while keeping prefix images alive.
func TestPartialFramesIncrementalCaret(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	ref, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer ref.Release()

	atlas := scene.NewAtlas(1, 256, 256)
	mx, my, _ := atlas.Alloc(8, 12)
	glyphPix := make([]byte, 96)
	for i := range glyphPix {
		glyphPix[i] = 200
	}
	atlas.Put(mx, my, 8, 12, glyphPix, 8)
	imgPix := make([]byte, 24*24*4)
	for i := range imgPix {
		imgPix[i] = byte((i * 13) | 0x80)
	}
	img := scene.NewImageRGBA(24, 24, imgPix)

	s := &scene.Scene{
		Width: 640, Height: 480, MaskAtlas: atlas, ID: 1,
		Stops: []scene.Stop{{Color: scene.Color{R: 250, G: 250, B: 250, A: 255}, At: 0}, {Color: scene.Color{R: 250, G: 250, B: 250, A: 0}, At: 1}},
	}
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 640, H: 480}, Color: scene.Color{R: 250, G: 250, B: 250, A: 255}})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{X: 20, Y: 20, W: 24, H: 24}, Image: img, Src: scene.Rect{W: 24, H: 24}})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 60, Y: 40, W: 500, H: 380}})
	for row := range 16 {
		start := int32(len(s.Glyphs))
		for col := range 30 {
			s.Glyphs = append(s.Glyphs, scene.Glyph{
				X: float32(70 + col*10), Y: float32(60 + row*20),
				W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
				Color: scene.Color{R: 25, G: 25, B: 30, A: 255},
			})
		}
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
	}
	opStart := len(s.Ops)
	caretIdx := len(s.Ops)
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 180, Y: 160, W: 2, H: 18}, Radii: scene.Uniform(1), Color: scene.Color{R: 15, G: 110, B: 240, A: 255}})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 60, Y: 40, W: 500, H: 20}, Paint: scene.PaintLinearStops, Gradient: [4]float32{60, 40, 60, 60}, Stops: [2]int32{0, 2}})
	opEnd := len(s.Ops)
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 570, Y: 20, W: 50, H: 20}, Radii: scene.Uniform(4), Color: scene.Color{R: 80, G: 90, B: 110, A: 255}})

	edit := scene.PartEdit{
		OpStart: opStart, OldOpEnd: opEnd, NewOpEnd: opEnd,
		GlyphMark: len(s.Glyphs), StopMark: len(s.Stops),
	}

	r.cur, r.wide = &r.formats[0], false
	var targets [3]id
	pool(func() {
		for i := range targets {
			targets[i] = r.newReadbackTexture(s.Width, s.Height, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead)
		}
	})
	defer pool(func() {
		r.waitLast()
		for i := range targets {
			release(&targets[i])
		}
	})

	readTex := func(tex id) []byte {
		cb := send(r.queue, "commandBuffer")
		blit := send(cb, "blitCommandEncoder")
		send(blit, "synchronizeResource:", tex)
		send(blit, "endEncoding")
		send(cb, "commit")
		send(cb, "waitUntilCompleted")
		raw := make([]byte, s.Width*s.Height*4)
		msgGetBytes(tex, sel("getBytes:bytesPerRow:fromRegion:mipmapLevel:"), unsafe.Pointer(&raw[0]), uint(s.Width*4),
			mtlRegion{W: uint(s.Width), H: uint(s.Height), D: 1}, 0)
		return raw
	}

	const totalFrames = 260 // > sweep() 240-frame threshold
	partial := 0
	for step := range totalFrames {
		if step > 0 {
			s.Ops[caretIdx].Color.A = byte(1 + (step*17)%254)
			edit.BaseID = s.ID
			s.ID = uint64(step + 2)
			s.Partial = edit
		}
		var got []byte
		var err error
		pool(func() {
			r.waitLast()
			r.cur = &r.formats[0]
			tex := targets[step%len(targets)]
			damage, known := r.diff.Diff(s)
			cb, e := r.drawFrame(s, damage, known, func() id { return tex })
			if e != nil {
				err = e
				return
			}
			send(cb, "commit")
			send(cb, "waitUntilCompleted")
			got = readTex(tex)
		})
		if err != nil {
			t.Fatal(err)
		}
		if step > 0 && r.drawState.b.IncrementalBatch() == 0 {
			t.Fatalf("step %d: expected incremental build", step)
		}
		if step >= 3 {
			if r.redrew != redrewPart {
				t.Fatalf("step %d: redrew = %v, want partial", step, r.redrew)
			}
			partial++
			if len(r.part.Instances) >= len(r.drawState.b.Instances)/10 {
				t.Fatalf("step %d: compacted %d / %d instances", step, len(r.part.Instances), len(r.drawState.b.Instances))
			}
		}
		// Spot-check byte-for-byte equality across all buffers early, mid-way, and past sweep().
		if step < 10 || step%50 == 0 || step == totalFrames-1 {
			want := renderRaw(t, ref, s, false)
			if n := differing(got, want); n != 0 {
				t.Fatalf("step %d: %d bytes differ from whole render (bounds=%v)", step, n, r.part.Bounds())
			}
		}
	}
	if partial != totalFrames-3 {
		t.Fatalf("partial = %d, want %d", partial, totalFrames-3)
	}
	r.sweep()
	if len(r.images) == 0 {
		t.Fatal("prefix image was incorrectly evicted by sweep()")
	}
}
