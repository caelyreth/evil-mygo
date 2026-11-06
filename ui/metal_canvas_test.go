package ui

import "testing"

func TestMetalCanvasIsAnImage(t *testing.T) {
	c := NewMetalCanvas(12, 8, nil)
	if w, h := c.Size(); w != 12 || h != 8 {
		t.Fatalf("canvas size is %d×%d, want 12×8", w, h)
	}
	if w, h := ImageSize(c.Bitmap()); w != 12 || h != 8 {
		t.Fatalf("bitmap size is %v×%v, want 12×8", w, h)
	}
	if c.Bitmap().smaller(4) != c.img {
		t.Fatal("a Metal canvas made a CPU downsampled level")
	}
}

func TestMetalCanvasResize(t *testing.T) {
	c := NewMetalCanvas(12, 8, nil)
	version := c.img.Version()
	c.Resize(24, 16)
	if w, h := c.Size(); w != 24 || h != 16 {
		t.Fatalf("resized canvas = %d×%d, want 24×16", w, h)
	}
	if w, h := c.Bitmap().Size(); w != 24 || h != 16 {
		t.Fatalf("resized bitmap = %d×%d, want 24×16", w, h)
	}
	if c.img.Version() == version {
		t.Fatal("resize did not invalidate the canvas")
	}
}

func TestMetalCanvasEffectUsesTheWholeTarget(t *testing.T) {
	c := NewMetalCanvas(64, 32, nil)
	tt := NewTester(func(ctx *Context) {
		Box(ctx).Size(20, 10).Effect(c.Effect())
	}, 20, 10)
	for _, op := range tt.rt.scene.Ops {
		if op.Image == c.img {
			if op.Src.W != 64 || op.Src.H != 32 {
				t.Fatalf("target source is %v, want 64×32", op.Src)
			}
			return
		}
	}
	t.Fatal("the Metal target was not painted")
}
