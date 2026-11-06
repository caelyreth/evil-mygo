//go:build darwin

package webgpu

import (
	"context"
	"testing"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"
)

func TestOpenAndOffscreenClear(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed; run go run github.com/go-webgpu/webgpu/cmd/setup@v0.5.5 ./lib")
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	if g.Device() == nil || g.Queue() == nil || g.Adapter() == nil {
		t.Fatal("Open returned incomplete WebGPU state")
	}
	if info := g.AdapterInfo(); info.BackendType != wgpu.BackendTypeMetal {
		t.Fatalf("adapter backend = %v, want Metal", info.BackendType)
	}

	const width, height = 64, 1
	texture, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Label: "clear target",
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
	defer texture.Release()
	view, err := texture.CreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()

	const size = width * height * 4
	readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{
		Label: "clear readback",
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Release()

	encoder, err := g.Device().CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "clear"})
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "clear pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: .25, G: .5, B: .75, A: 1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pass.End()
	pass.Release()
	encoder.CopyTextureToBuffer(texture, readback, []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: texture, Aspect: wgpu.TextureAspectAll},
		BufferLayout: wgpu.ImageDataLayout{
			BytesPerRow:  size,
			RowsPerImage: 1,
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
	if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatal(err)
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	if ptr == nil {
		t.Fatal("mapped readback buffer has no bytes")
	}
	// go-webgpu v0.5.5's MappedRange.Bytes calls wgpuBufferGetMapState,
	// which wgpu-native v29 leaves unimplemented. GetMappedRange is valid
	// once Map has succeeded.
	pixels := unsafe.Slice((*byte)(ptr), size)
	for i := 0; i < len(pixels); i += 4 {
		if pixels[i] != 64 || pixels[i+1] != 128 || pixels[i+2] != 191 || pixels[i+3] != 255 {
			t.Fatalf("pixel %d = %v, want [64 128 191 255]", i/4, pixels[i:i+4])
		}
	}
}
