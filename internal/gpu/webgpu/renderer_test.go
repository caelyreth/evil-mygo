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

func TestRendererDrawsImageBatch(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()

	const width, height = 64, 24
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	pix := []byte{
		12, 34, 56, 255, 12, 34, 56, 255,
		12, 34, 56, 255, 12, 34, 56, 255,
	}
	img := scene.NewImageRGBA(2, 2, pix)
	s := &scene.Scene{
		Width: width, Height: height,
		Ops: []scene.Op{{
			Kind: scene.OpImage, Image: img,
			Rect: scene.Rect{W: width, H: height},
			Src:  scene.Rect{W: 2, H: 2},
		}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	pixel := unsafe.Slice((*byte)(ptr), size)[(height/2*width+width/2)*4:][:4]
	if pixel[0] != 12 || pixel[1] != 34 || pixel[2] != 56 || pixel[3] != 255 {
		t.Fatalf("image pixel = %v, want [12 34 56 255]", pixel)
	}
}

func TestRendererDrawsMaskGlyph(t *testing.T) {
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	atlas := scene.NewAtlas(1, 32, 32)
	x, y, ok := atlas.Alloc(16, 16)
	if !ok {
		t.Fatal("cannot allocate glyph mask")
	}
	mask := make([]byte, 16*16)
	for i := range mask {
		mask[i] = 255
	}
	atlas.Put(x, y, 16, 16, mask, 16)
	glyphs := []scene.Glyph{{
		X: 8, Y: 8, W: 16, H: 16,
		U: uint16(x), V: uint16(y), UW: 16, VH: 16,
		Color: scene.Color{R: 220, G: 30, B: 20, A: 255},
	}}
	s := &scene.Scene{
		Width: width, Height: height,
		MaskAtlas: atlas, Glyphs: glyphs,
		Ops: []scene.Op{{Kind: scene.OpGlyphs, Start: 0, End: 1}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	pixel := pixels[(16*width+16)*4:][:4]
	if pixel[0] != 220 || pixel[1] != 30 || pixel[2] != 20 || pixel[3] != 255 {
		t.Fatalf("glyph pixel = %v, want [220 30 20 255]", pixel)
	}
}

func TestRendererDrawsLinearGradient(t *testing.T) {
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()
	s := &scene.Scene{
		Width: width, Height: height,
		Ops: []scene.Op{{
			Kind: scene.OpFill, Rect: scene.Rect{W: width, H: height},
			Color:  scene.Color{R: 255, A: 255},
			Color2: scene.Color{B: 255, A: 255},
			Paint:  scene.PaintLinear, Gradient: [4]float32{0, 0, width, 0},
			Border: scene.Uniform(2), BorderColor: scene.Color{G: 255, A: 255},
		}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	left := pixels[(16*width+2)*4:][:4]
	right := pixels[(16*width+61)*4:][:4]
	border := pixels[(16*width+1)*4:][:4]
	if left[0] <= left[2] || right[2] <= right[0] {
		t.Fatalf("gradient endpoints left=%v right=%v", left, right)
	}
	if border[1] <= border[0] || border[1] <= border[2] {
		t.Fatalf("border pixel = %v, want green", border)
	}
}

func TestRendererAppliesClipMask(t *testing.T) {
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	s := &scene.Scene{
		Width: width, Height: height,
		Clear: scene.Color{R: 4, G: 8, B: 12, A: 255},
		Ops: []scene.Op{
			{Kind: scene.OpPushClip, Rect: scene.Rect{X: 8, Y: 8, W: 32, H: 16}, Radii: [4]float32{8, 8, 8, 8}},
			{Kind: scene.OpFill, Rect: scene.Rect{X: 8, Y: 8, W: 32, H: 16}, Color: scene.Color{R: 220, G: 30, B: 20, A: 255}, Transform: scene.Scaling(1.01, 1)},
			{Kind: scene.OpPopClip},
		},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	if len(renderer.root.builder.Batches) == 0 || renderer.root.builder.Batches[0].ClipMask == 0 {
		t.Fatal("scene did not exercise a clip mask")
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	inside := pixels[(16*width+16)*4:][:4]
	outside := pixels[(16*width+48)*4:][:4]
	if inside[0] != 220 || inside[1] != 30 || inside[2] != 20 {
		t.Fatalf("clipped fill pixel = %v, want [220 30 20]", inside)
	}
	if outside[0] != 4 || outside[1] != 8 || outside[2] != 12 {
		t.Fatalf("outside pixel = %v, want clear [4 8 12]", outside)
	}
}

func TestRendererDrawsBlurredShadow(t *testing.T) {
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()
	s := &scene.Scene{
		Width: width, Height: height,
		Ops: []scene.Op{{
			Kind:  scene.OpShadow,
			Rect:  scene.Rect{X: 20, Y: 8, W: 24, H: 16},
			Radii: [4]float32{4, 4, 4, 4},
			Color: scene.Color{R: 180, G: 180, B: 180, A: 255},
			Blur:  8,
		}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	inside := pixels[(16*width+32)*4:][:4]
	edge := pixels[(16*width+16)*4:][:4]
	if inside[0] < 80 {
		t.Fatalf("shadow core = %v, want a bright shadow", inside)
	}
	if edge[0] == 0 {
		t.Fatalf("blurred shadow edge is empty at %v", edge)
	}
}

func TestRendererDrawsNestedCanvas(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()

	const width, height = 64, 48
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
	renderer, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Release()

	canvas := scene.NewCanvas(16, 16)
	canvas.Scene.Clear = scene.Color{R: 200, A: 255}
	canvas.Scene.Ops = append(canvas.Scene.Ops, scene.Op{
		Kind: scene.OpFill, Rect: scene.Rect{W: 16, H: 16},
		Color: scene.Color{B: 220, A: 255},
	})
	s := &scene.Scene{
		Width: width, Height: height,
		Clear: scene.Color{G: 10, A: 255},
		Ops: []scene.Op{{
			Kind: scene.OpImage, Image: canvas,
			Rect: scene.Rect{X: 8, Y: 8, W: 16, H: 16},
			Src:  scene.Rect{W: 16, H: 16},
		}},
	}
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		t.Fatal(err)
	}
	g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
	if err := renderer.Render(s, encoder, view); err != nil {
		t.Fatal(err)
	}
	const size = width * height * 4
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
			BytesPerRow: width * 4, RowsPerImage: height,
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
	inside := pixels[(16*width+16)*4:][:4]
	outside := pixels[(40*width+4)*4:][:4]
	if inside[0] != 0 || inside[1] != 0 || inside[2] != 220 || inside[3] != 255 {
		t.Fatalf("canvas pixel = %v, want [0 0 220 255]", inside)
	}
	if outside[0] != 0 || outside[1] != 10 || outside[2] != 0 {
		t.Fatalf("outside pixel = %v, want the root clear color", outside)
	}
}
