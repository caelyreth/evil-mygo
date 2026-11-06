package ui

import (
	"testing"
	"unsafe"
)

func TestMetalTextureIsAnImage(t *testing.T) {
	var device, texture unsafe.Pointer
	tx := NewMetalTexture(9, 7, device, texture, "owner")
	if w, h := tx.Size(); w != 9 || h != 7 {
		t.Fatalf("texture size is %d×%d, want 9×7", w, h)
	}
	if w, h := ImageSize(tx.Bitmap()); w != 9 || h != 7 {
		t.Fatalf("bitmap size is %v×%v, want 9×7", w, h)
	}
	if tx.Bitmap().smaller(4) != tx.img {
		t.Fatal("an imported Metal texture made a CPU downsampled level")
	}
	before := tx.img.Version()
	tx.Invalidate()
	if tx.img.Version() == before {
		t.Fatal("Invalidate did not change the image version")
	}
}
