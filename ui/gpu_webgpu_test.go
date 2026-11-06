//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package ui

import (
	stdcontext "context"
	"testing"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu/webgpu"
	"github.com/egoist/mygo/internal/scene"
)

func TestWebGPUDrawsNativeUILayer(t *testing.T) {
	if _, ok := webgpu.RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(16).Children(func() {
			Box(c).Fill().Background(RGB(220, 32, 24)).Width(72).Height(40)
			Text(c, "WebGPU native UI").FontSize(18).TextColor(RGB(20, 20, 20))
		})
	}, 180, 100)
	defer layer.Close()

	g, err := webgpu.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	renderer, err := webgpu.NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	const width, height = 180, 100
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size:          wgpu.Extent3D{Width: width, Height: height, DepthOrArrayLayers: 1},
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
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()
	root := &scene.Scene{
		Width: width, Height: height,
		Clear: scene.Color{A: 255},
		Ops: []scene.Op{{
			Kind: scene.OpImage, Image: layer.img,
			Rect: scene.Rect{W: width, H: height},
			Src:  scene.Rect{W: width, H: height},
		}},
	}
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := renderer.Render(root, encoder, view); err != nil {
		t.Fatal(err)
	}
	const rowBytes = (width*4 + 255) &^ 255
	const size = rowBytes * height
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
			BytesPerRow: rowBytes, RowsPerImage: height,
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
	if typ, message, err := g.Device().PopErrorScopeAsync(g.Instance()); err != nil {
		t.Fatal(err)
	} else if typ != wgpu.ErrorTypeNoError {
		t.Fatalf("WebGPU validation error %v: %s", typ, message)
	}
	command.Release()
	if err := readback.Map(stdcontext.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatal(err)
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	if ptr == nil {
		t.Fatal("no mapped pixels")
	}
	pixels := unsafe.Slice((*byte)(ptr), size)
	px := pixels[28*rowBytes+28*4:][:4]
	if px[0] < 180 || px[1] > 70 || px[2] > 70 || px[3] == 0 {
		t.Fatalf("native UI pixel = %v, want the red box", px)
	}
}

func TestWebGPUTextureSync(t *testing.T) {
	tex := NewWebGPUTexture(1, 1, nil, nil, nil)
	called := false
	tex.SetSync(WebGPUTextureSync{
		Wait: func(device *wgpu.Device, encoder *wgpu.CommandEncoder, texture *wgpu.Texture) error {
			called = true
			return nil
		},
	})
	if tex.img.External.Sync.WaitObject == nil {
		t.Fatal("SetSync did not install WaitObject")
	}
	if err := tex.img.External.Sync.WaitObject(nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("WaitObject did not call Wait")
	}
	tex.SetSync(WebGPUTextureSync{})
	if tex.img.External.Sync.WaitObject != nil {
		t.Fatal("zero SetSync did not clear WaitObject")
	}
}
