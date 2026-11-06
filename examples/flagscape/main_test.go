//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"testing"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu/webgpu"
	"github.com/egoist/mygo/ui"
)

func TestWebGPUSceneCompilesAndEncodes(t *testing.T) {
	g, err := webgpu.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	source, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Label:         "Flagscape test source",
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
		Size:          wgpu.Extent3D{Width: 32, Height: 24, DepthOrArrayLayers: 1},
		Format:        wgpu.TextureFormatRGBA8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	sourceView, err := source.CreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceView.Release()
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Label:         "Flagscape test target",
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size:          wgpu.Extent3D{Width: 32, Height: 24, DepthOrArrayLayers: 1},
		Format:        wgpu.TextureFormatRGBA8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()
	a := newApp()
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := a.draw(ui.WebGPUFrame{
		Device: g.Device(), Encoder: encoder, Texture: target,
		Source: sourceView, SourceTexture: source,
		Width: 32, Height: 24, SourceWidth: 32, SourceHeight: 24,
	}); err != nil {
		t.Fatal(err)
	}
	command, err := encoder.Finish(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Queue().Submit(command); err != nil {
		t.Fatal(err)
	}
	command.Release()
	g.Device().Poll(false)
	if typ, message, err := g.Device().PopErrorScopeAsync(g.Instance()); err != nil {
		t.Fatal(err)
	} else if typ != wgpu.ErrorTypeNoError {
		t.Fatalf("WebGPU validation error %v: %s", typ, message)
	}
}

func TestPanelUVCenter(t *testing.T) {
	a := newApp()
	u, v, ok := a.panelUV(800, 500, 1600, 1000)
	if !ok {
		t.Fatal("center ray missed the panel")
	}
	if u < 0.45 || u > 0.55 || v < 0.5 || v > 0.7 {
		t.Fatalf("center UV = %v, %v", u, v)
	}
}
