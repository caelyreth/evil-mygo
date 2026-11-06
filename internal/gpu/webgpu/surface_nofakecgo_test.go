//go:build darwin && nofakecgo

package webgpu

import (
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/go-webgpu/webgpu/wgpu"
)

func TestCAMetalLayerSurface(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()

	layer := objc.Send[objc.ID](objc.ID(objc.GetClass("CAMetalLayer")), objc.RegisterName("new"))
	if layer == 0 {
		t.Fatal("cannot create CAMetalLayer")
	}
	defer objc.Send[objc.ID](layer, objc.RegisterName("release"))
	s, err := g.NewSurface(uintptr(layer))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Release()
	if s.Format() == wgpu.TextureFormatUndefined {
		t.Fatal("surface has no usable presentation format")
	}
	if err := s.Configure(320, 200); err != nil {
		t.Fatal(err)
	}
	if w, h := s.Size(); w != 320 || h != 200 {
		t.Fatalf("surface size = %d×%d, want 320×200", w, h)
	}
	texture, _, err := s.Acquire()
	if err != nil {
		t.Fatalf("acquire surface texture: %v", err)
	}
	if texture == nil || texture.Texture == nil {
		t.Fatal("surface returned no texture")
	}
	view, err := texture.Texture.CreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: .1, G: .2, B: .3, A: 1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pass.End()
	pass.Release()
	command, err := encoder.Finish(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Queue().Submit(command); err != nil {
		t.Fatal(err)
	}
	command.Release()
	encoder.Release()
	view.Release()
	texture.Texture.Release()
	if err := s.Present(); err != nil {
		t.Fatal(err)
	}
}
