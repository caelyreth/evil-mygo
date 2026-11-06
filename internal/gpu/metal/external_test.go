//go:build darwin

package metal

import (
	"errors"
	"testing"
	"unsafe"

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
	var events []string
	img.External.Sync = scene.ExternalTextureSync{
		Wait: func(device, commandBuffer, texture unsafe.Pointer) error {
			if device != ptr(r.device) || commandBuffer == nil || texture != ptr(tex) {
				t.Fatalf("wait saw device %p, command buffer %p, texture %p", device, commandBuffer, texture)
			}
			events = append(events, "wait")
			return nil
		},
		Signal: func(device, commandBuffer, texture unsafe.Pointer) error {
			if device != ptr(r.device) || commandBuffer == nil || texture != ptr(tex) {
				t.Fatalf("signal saw device %p, command buffer %p, texture %p", device, commandBuffer, texture)
			}
			events = append(events, "signal")
			return nil
		},
	}
	s := &scene.Scene{Width: n, Height: n}
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: n, H: n}, Image: img, Src: scene.Rect{W: n, H: n}})
	got, err := r.renderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != "wait" || events[1] != "signal" {
		t.Fatalf("external synchronization events = %v, want [wait signal]", events)
	}
	for i := 0; i < n*n; i++ {
		p := got[4*i : 4*i+4]
		if p[0] != 0 || p[1] != 0 || p[2] != 255 || p[3] != 255 {
			t.Fatalf("external texture pixel %d is %v, want premultiplied red", i, p)
		}
	}
}

func TestExternalTextureWaitErrorFailsTheFrame(t *testing.T) {
	r, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer r.Release()
	const n = 2
	var tex id
	pool(func() { tex = r.newTexture(n, n, pixelFormatRGBA8Unorm, usageShaderRead, make([]byte, 4*n*n), n*4) })
	if tex == 0 {
		t.Fatal("cannot make an external texture")
	}
	defer pool(func() { release(&tex) })
	want := errors.New("not ready")
	img := scene.NewExternalImage(n, n, ptr(r.device), ptr(tex), nil)
	img.External.Sync.Wait = func(device, commandBuffer, texture unsafe.Pointer) error {
		return want
	}
	s := &scene.Scene{Width: n, Height: n}
	s.Ops = append(s.Ops, scene.Op{Kind: scene.OpImage, Rect: scene.Rect{W: n, H: n}, Image: img, Src: scene.Rect{W: n, H: n}})
	if _, err := r.renderOffscreen(s); !errors.Is(err, want) {
		t.Fatalf("render error = %v, want %v", err, want)
	}
	img.External.Sync.Wait = nil
	if _, err := r.renderOffscreen(s); err != nil {
		t.Fatalf("renderer did not recover after the producer became ready: %v", err)
	}
}
