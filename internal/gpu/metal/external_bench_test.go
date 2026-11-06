//go:build darwin

package metal

import (
	"testing"
	"unsafe"

	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkRenderExternalTexture samples an application-owned texture
// with acquire/signal hooks every frame. It measures the same-device
// compatibility path, not pixels uploaded through Go.
func BenchmarkRenderExternalTexture(b *testing.B) {
	const w, h = 1920, 1080
	img := scene.NewExternalImage(512, 512, nil, nil, nil)
	img.External.Sync = scene.ExternalTextureSync{
		Wait: func(device, commandBuffer, texture unsafe.Pointer) error { return nil },
		Signal: func(device, commandBuffer, texture unsafe.Pointer) error {
			return nil
		},
	}
	s := &scene.Scene{
		Width: w, Height: h,
		Clear: scene.Color{A: 255},
		Ops: []scene.Op{{
			Kind: scene.OpImage, Image: img,
			Rect: scene.Rect{W: w, H: h}, Src: scene.Rect{W: 512, H: 512},
		}},
	}
	var tex id
	benchRenderWith(b, s, func(r *Renderer) {
		pool(func() {
			tex = r.newTexture(512, 512, pixelFormatRGBA8Unorm, usageShaderRead, make([]byte, 4*512*512), 512*4)
		})
		if tex == 0 {
			b.Fatal("cannot make an external texture")
		}
		img.External.Device, img.External.Texture = ptr(r.device), ptr(tex)
	})
	b.Cleanup(func() { pool(func() { release(&tex) }) })
}
