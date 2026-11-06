//go:build darwin && (cgo || nofakecgo)

package webgpu

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

func TestSurfaceWideFormat(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_NOW); err != nil {
		t.Fatal(err)
	}
	layer := objc.Send[objc.ID](objc.ID(objc.GetClass("CAMetalLayer")), objc.RegisterName("new"))
	if layer == 0 {
		t.Fatal("cannot create CAMetalLayer")
	}
	defer objc.Send[objc.ID](layer, objc.RegisterName("release"))
	r, err := NewNative(uintptr(layer))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	if r.surface.Wide() {
		t.Fatal("new surface started wide")
	}
	if !r.SetWide(true) {
		t.Skip("CAMetalLayer surface has no RGBA16Float format")
	}
	if !r.surface.Wide() || r.renderer.format != wgpu.TextureFormatRGBA16Float {
		t.Fatalf("wide surface = %v, renderer format = %v", r.surface.Wide(), r.renderer.format)
	}
	if err := r.Render(&scene.Scene{
		Width: 64, Height: 32,
		Clear: scene.Color{R: 8, G: 12, B: 16, A: 255},
		Ops: []scene.Op{{
			Kind: scene.OpFill, Rect: scene.Rect{X: 4, Y: 4, W: 56, H: 24},
			Color: scene.Color{R: 240, G: 180, B: 40, A: 255},
		}},
	}); err != nil {
		t.Fatalf("wide surface render: %v", err)
	}
	if !r.SetWide(false) {
		t.Fatal("could not return surface to sRGB")
	}
	if r.surface.Wide() || r.renderer.format == wgpu.TextureFormatRGBA16Float {
		t.Fatalf("sRGB surface = %v, renderer format = %v", r.surface.Wide(), r.renderer.format)
	}
}
