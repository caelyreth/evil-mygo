//go:build darwin && nofakecgo

package webgpu

import (
	"testing"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/scene"
)

func TestNativeRendererPresentsFrames(t *testing.T) {
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
	draw := func(width, height int) error {
		return r.Render(&scene.Scene{
			Width: width, Height: height,
			Clear: scene.Color{R: 8, G: 12, B: 16, A: 255},
			Ops: []scene.Op{{
				Kind:  scene.OpFill,
				Rect:  scene.Rect{X: 4, Y: 4, W: float32(width - 8), H: float32(height - 8)},
				Radii: scene.Uniform(4),
				Color: scene.Color{R: 40, G: 120, B: 80, A: 255},
			}},
		})
	}
	if err := draw(80, 48); err != nil {
		t.Fatal(err)
	}
	if err := draw(96, 64); err != nil {
		t.Fatalf("second frame after resize: %v", err)
	}
}
