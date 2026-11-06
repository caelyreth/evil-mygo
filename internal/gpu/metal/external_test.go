//go:build darwin

package metal

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

func TestExternalTextureIsSampledWithoutUpload(t *testing.T) {
	r := &Renderer{device: 123}
	img := scene.NewExternalImage(8, 6, ptr(123), ptr(456), nil)
	if got := r.imageTexture(img); got != 456 {
		t.Fatalf("external texture resolved to %d, want 456", got)
	}
	other := scene.NewExternalImage(8, 6, ptr(999), ptr(456), nil)
	if got := r.imageTexture(other); got != 0 {
		t.Fatalf("texture from another device resolved to %d, want 0", got)
	}
}

func TestExternalTextureDraws(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	const n = 4
	pix := make([]byte, 4*n*n)
	for i := 0; i < n*n; i++ {
		pix[4*i], pix[4*i+3] = 255, 255 // red premultiplied
	}
	var tex id
	pool(func() { tex = r.newTexture(n, n, pixelFormatRGBA8Unorm, usageShaderRead, pix, n*4) })
	if tex == 0 {
		t.Fatal("cannot make an external texture")
	}
	defer pool(func() { release(&tex) })
	img := scene.NewExternalImage(n, n, ptr(r.device), ptr(tex), nil)
	s := &scene.Scene{Width: n, Height: n}
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: n, H: n}, Image: img, Src: scene.Rect{W: n, H: n}})
	got, err := r.renderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n*n; i++ {
		p := got[4*i : 4*i+4]
		if p[0] != 0 || p[1] != 0 || p[2] != 255 || p[3] != 255 {
			t.Fatalf("external texture pixel %d is %v, want premultiplied red", i, p)
		}
	}
}
