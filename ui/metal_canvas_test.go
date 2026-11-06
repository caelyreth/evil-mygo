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
