//go:build darwin

package webgpu

import (
	"context"
	"testing"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

func TestRendererDrawsRoundedFill(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()

	const width, height = 64, 32
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Label: "fill target",
		Usage: wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size: wgpu.Extent3D{
			Width: width, Height: height, DepthOrArrayLayers: 1,
		},
		Format:        wgpu.TextureFormatRGBA8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	view, err := target.CreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	s := &scene.Scene{
		Width: width, Height: height,
		Clear: scene.Color{R: 4, G: 8, B: 12, A: 255},
		Ops: []scene.Op{{
			Kind:  scene.OpFill,
			Rect:  scene.Rect{X: 8, Y: 8, W: 48, H: 16},
			Radii: [4]float32{4, 4, 4, 4},
			Color: scene.Color{R: 255, G: 20, B: 10, A: 255},
		}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const bytesPerRow = width * 4
	const size = bytesPerRow * height
	readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Release()
	encoder.CopyTextureToBuffer(target, readback, []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: target, Aspect: wgpu.TextureAspectAll},
		BufferLayout: wgpu.ImageDataLayout{
			BytesPerRow: bytesPerRow, RowsPerImage: height,
		},
		Size: wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
	}})
	command, err := encoder.Finish(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Queue().Submit(command); err != nil {
		t.Fatal(err)
	}
	if typ, message, err := g.Device().PopErrorScopeAsync(g.instance); err != nil {
		t.Fatal(err)
	} else if typ != wgpu.ErrorTypeNoError {
		t.Fatalf("WebGPU validation error %v: %s", typ, message)
	}
	command.Release()
	encoder.Release()
	if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatal(err)
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	if ptr == nil {
		t.Fatal("no mapped pixels")
	}
	pixels := unsafe.Slice((*byte)(ptr), size)
	at := func(x, y int) []byte {
		o := y*bytesPerRow + x*4
		return pixels[o : o+4]
	}
	if p := at(40, 16); p[0] != 255 || p[1] != 20 || p[2] != 10 || p[3] != 255 {
		t.Fatalf("fill pixel = %v, want [255 20 10 255]", p)
	}
	if p := at(0, 0); p[0] != 4 || p[1] != 8 || p[2] != 12 || p[3] != 255 {
		t.Fatalf("clear pixel = %v, want [4 8 12 255]", p)
	}
}
