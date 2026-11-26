//go:build darwin && !ios

package metal

import (
	"runtime"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// Canvases draw into textures as the CPU renderer draws them into pixels,
// once each: the second frame draws none anew, and shows them the same.
func TestCanvases(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	s := gputest.CanvasScene()
	for frame := range 2 {
		pix, err := r.renderOffscreen(s)
		if err != nil {
			t.Fatal(err)
		}
		gputest.Compare(t, "metal-canvas", pix, s.Width*4, s)
		if frame == 0 {
			s.Canvases = nil // drawn: only shown from now on
		}
	}
	if len(r.canvases) != 3 {
		t.Errorf("%d canvas textures, want 3", len(r.canvases))
	}
}

// A canvas that keeps the bitmaps of its glyphs (scene.Scene.Own) draws
// them from its own atlases, not from the frame's, which hold others, in
// the frame that draws it, in one that only shows it and in one that
// draws it anew; so does one with a mask atlas of its own alone.
func TestOwnedCanvas(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	for _, color := range []bool{true, false} {
		s := gputest.OwnedCanvasScene(color)
		for frame := range gputest.OwnedCanvasFrames {
			pix, err := r.renderOffscreen(s)
			if err != nil {
				t.Fatal(err)
			}
			gputest.Compare(t, "metal-owned-canvas", pix, s.Width*4, s)
			gputest.OwnedCanvasNext(s, frame)
		}
	}
}

// A transient canvas's texture stays while frames draw it, and goes once
// they stop, though the canvas is still about; another canvas's stays
// until it is collected.
func TestTransientCanvas(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()
	layer, kept := scene.NewCanvas(64, 64), scene.NewCanvas(64, 64)
	layer.Transient = true
	s := &scene.Scene{Width: 64, Height: 64}
	s.Canvases = []*scene.Image{layer, kept}
	if _, err := r.renderOffscreen(s); err != nil {
		t.Fatal(err)
	}
	r.sweep()
	if r.canvases[layer.ID()] == nil || r.canvases[kept.ID()] == nil {
		t.Fatal("a sweep after the frame that drew them freed a canvas's texture")
	}
	r.frame++ // a frame that draws neither
	r.sweep()
	if r.canvases[layer.ID()] != nil || r.canvases[kept.ID()] == nil {
		t.Errorf("transient canvas kept %v, other kept %v", r.canvases[layer.ID()] != nil, r.canvases[kept.ID()] != nil)
	}
	runtime.KeepAlive(layer)
	runtime.KeepAlive(kept)
}

// TestReleasedCanvasReaped verifies that explicitly releasing a canvas drops
// its Metal texture on TrimIdleImages or the next presented frame rather than
// waiting 120 frames and Go garbage collection.
func TestReleasedCanvasReaped(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		t.Skip("no Metal:", err)
	}
	defer r.Release()

	kept := scene.NewCanvas(64, 64)
	s := &scene.Scene{Width: 64, Height: 64, Canvases: []*scene.Image{kept}}
	if _, err := r.renderOffscreen(s); err != nil {
		t.Fatal(err)
	}
	if r.canvases[kept.ID()] == nil {
		t.Fatal("canvas texture missing after renderOffscreen")
	}

	// Releasing marks the image released and drops its scene; TrimIdleImages
	// reaps it even while caller keeps a live Go reference.
	kept.Release()
	if !kept.Released() || kept.Scene != nil {
		t.Fatalf("after Release(): released=%v Scene=%v", kept.Released(), kept.Scene)
	}
	r.TrimIdleImages()
	if r.canvases[kept.ID()] != nil {
		t.Error("released canvas texture still present after TrimIdleImages")
	}

	// Also test layer-backed Render() frame reaping without waiting sweepEvery.
	var layer id
	pool(func() { layer = send(class("CALayer"), "new") })
	if layer == 0 {
		t.Skip("no Core Animation")
	}
	defer pool(func() { release(&layer) })
	lr, err := New(uintptr(layer))
	if err != nil {
		unavailable(t, err)
	}
	defer lr.Release()

	c2 := scene.NewCanvas(64, 64)
	s2 := &scene.Scene{Width: 64, Height: 64, Canvases: []*scene.Image{c2}}
	if err := lr.Render(s2); err != nil {
		t.Fatal(err)
	}
	if lr.canvases[c2.ID()] == nil {
		t.Fatal("layer canvas texture missing after Render")
	}
	c2.Release()
	s2.Canvases = nil
	s2.Ops = append(s2.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 8, H: 8}, Color: scene.Color{R: 1, A: 255}})
	if err := lr.Render(s2); err != nil {
		t.Fatal(err)
	}
	if lr.canvases[c2.ID()] != nil {
		t.Error("released canvas texture not reaped on next Render frame")
	}
	runtime.KeepAlive(kept)
	runtime.KeepAlive(c2)
}
