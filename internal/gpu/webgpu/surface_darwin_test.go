//go:build darwin

package webgpu

import (
	"context"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego/objc"
	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo/internal/scene"
)

func TestChooseSurfaceFormatPrefersNonSRGB(t *testing.T) {
	got, ok := chooseSurfaceFormat([]wgpu.TextureFormat{
		wgpu.TextureFormatBGRA8UnormSrgb,
		wgpu.TextureFormatRGBA8UnormSrgb,
		wgpu.TextureFormatBGRA8Unorm,
		wgpu.TextureFormatRGBA8Unorm,
	})
	if !ok || got != wgpu.TextureFormatBGRA8Unorm {
		t.Fatalf("surface format = %v, %v; want BGRA8Unorm", got, ok)
	}
}

func TestChooseSurfaceFormatFallsBackToSRGB(t *testing.T) {
	got, ok := chooseSurfaceFormat([]wgpu.TextureFormat{
		wgpu.TextureFormatRGBA8UnormSrgb,
		wgpu.TextureFormatBGRA8UnormSrgb,
	})
	if !ok || got != wgpu.TextureFormatBGRA8UnormSrgb {
		t.Fatalf("surface format = %v, %v; want BGRA8UnormSrgb", got, ok)
	}
}

func TestSurfaceStoresEightBitColorsDirectly(t *testing.T) {
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
	if got := s.Format(); got != wgpu.TextureFormatBGRA8Unorm {
		t.Fatalf("surface format = %v, want BGRA8Unorm", got)
	}
	const width, height = 64, 64
	if err := s.Configure(width, height); err != nil {
		t.Fatal(err)
	}
	texture, _, err := s.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if texture == nil || texture.Texture == nil {
		t.Fatal("surface returned no texture")
	}
	defer texture.Texture.Release()
	view, err := texture.Texture.CreateView(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer view.Release()
	renderer, err := NewRenderer(g, s.Format())
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Release()
	if err := renderer.Render(&scene.Scene{
		Width: width, Height: height,
		Clear: scene.Color{R: 21, G: 21, B: 20, A: 255},
	}, encoder, view); err != nil {
		t.Fatal(err)
	}
	const rowBytes = 256
	const size = rowBytes * height
	readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer readback.Release()
	encoder.CopyTextureToBuffer(texture.Texture, readback, []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: texture.Texture, Aspect: wgpu.TextureAspectAll},
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
	renderer.FinishFrame()
	if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		t.Fatal(err)
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	pixels := unsafe.Slice((*byte)(ptr), size)
	pixel := pixels[:4]
	if pixel[0] != 20 || pixel[1] != 21 || pixel[2] != 21 || pixel[3] != 255 {
		t.Fatalf("surface pixel = %v, want [20 21 21 255]", pixel)
	}
}
