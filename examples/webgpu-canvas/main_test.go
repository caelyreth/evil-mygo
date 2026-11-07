//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"context"
	"testing"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/ui"
)

func TestWebGPUCanvasDrawsPixels(t *testing.T) {
	g, err := webgpu.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	const width, height = 64, 64
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
	a := newApp()
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := a.draw(ui.WebGPUFrame{
		Device: g.Device(), Encoder: encoder, Texture: target,
		Width: width, Height: height,
	}); err != nil {
		t.Fatal(err)
	}
	const rowBytes = width * 4
	const size = rowBytes * height
	readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst, Size: size,
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
	command.Release()
	if typ, message, err := g.Device().PopErrorScopeAsync(g.Instance()); err != nil {
		t.Fatal(err)
	} else if typ != wgpu.ErrorTypeNoError {
		t.Fatalf("WebGPU validation error %v: %s", typ, message)
	}
	if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatal(err)
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	pixels := unsafe.Slice((*byte)(ptr), size)
	center := pixels[(height/2*width+width/2)*4:][:4]
	if center[1] <= 25 || center[0] == 5 && center[1] == 13 && center[2] == 15 && center[3] == 255 {
		t.Fatalf("canvas center = %v, want the shader color rather than the clear", center)
	}
}
