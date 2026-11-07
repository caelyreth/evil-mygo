//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/ui"
)

func TestAnimatedSceneGPU(t *testing.T) {
	if _, ok := webgpu.RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	g, err := webgpu.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	r := &sceneRenderer{model: newArchiveModel()}
	defer r.release()
	if unsafe.Sizeof(vertex{}) != 48 || unsafe.Sizeof(instance{}) != 32 || unsafe.Sizeof(material{}) != 32 || unsafe.Sizeof(globals{}) != 272 {
		t.Fatal("Go/WGSL memory layout drift")
	}
	width, height := 320, 200
	if os.Getenv("MYGO_RHINE_CAPTURE") != "" {
		width, height = 1280, 800
	}
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{Label: "Archive test output", Usage: wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size: wgpu.Extent3D{Width: uint32(width), Height: uint32(height), DepthOrArrayLayers: 1}, Format: wgpu.TextureFormatRGBA8Unorm, MipLevelCount: 1, SampleCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer target.Release()
	drawScene := func(shot float64) *image.RGBA {
		t.Helper()
		r.clock.elapsed = shot - shotStart
		encoder, err := g.Device().CreateCommandEncoder(nil)
		if err != nil {
			t.Fatal(err)
		}
		defer encoder.Release()
		g.Device().PushErrorScope(wgpu.ErrorFilterValidation)
		// No captured UI source: all pixels come from the scene's own GPU passes.
		if err := r.draw(ui.WebGPUFrame{Device: g.Device(), Encoder: encoder, Texture: target, Width: width, Height: height}); err != nil {
			t.Fatal(err)
		}
		stride := (width*4 + 255) / 256 * 256
		size := uint64(stride * height)
		readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{Label: "Test-only scene capture", Usage: wgpu.BufferUsageCopyDst | wgpu.BufferUsageMapRead, Size: size})
		if err != nil {
			t.Fatal(err)
		}
		defer readback.Release()
		encoder.CopyTextureToBuffer(target, readback, []wgpu.BufferTextureCopy{{
			TextureBase:  wgpu.ImageCopyTexture{Texture: target, Aspect: wgpu.TextureAspectAll},
			BufferLayout: wgpu.ImageDataLayout{BytesPerRow: uint32(stride), RowsPerImage: uint32(height)},
			Size:         wgpu.Extent3D{Width: uint32(width), Height: uint32(height), DepthOrArrayLayers: 1},
		}})
		command, err := encoder.Finish(nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := g.Queue().Submit(command); err != nil {
			command.Release()
			t.Fatal(err)
		}
		command.Release()
		g.Device().Poll(false)
		if typ, message, err := g.Device().PopErrorScopeAsync(g.Instance()); err != nil {
			t.Fatal(err)
		} else if typ != wgpu.ErrorTypeNoError {
			t.Fatalf("WebGPU validation %v: %s", typ, message)
		}
		if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
			t.Fatal(err)
		}
		defer readback.Unmap()
		pointer := readback.GetMappedRange(0, size)
		if pointer == nil {
			t.Fatal("capture did not map")
		}
		pixels := unsafe.Slice((*byte)(pointer), int(size))
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		minimum, maximum := byte(255), byte(0)
		for y := range height {
			copy(img.Pix[y*img.Stride:y*img.Stride+width*4], pixels[y*stride:y*stride+width*4])
			for x := range width {
				p := pixels[y*stride+x*4:]
				minimum, maximum = min(minimum, p[0]), max(maximum, p[0])
				if p[3] != 255 {
					t.Fatalf("transparent scene pixel at %d,%d", x, y)
				}
			}
		}
		if int(maximum)-int(minimum) < 40 {
			t.Fatalf("scene lacks geometry contrast: %d..%d", minimum, maximum)
		}
		if path := os.Getenv("MYGO_RHINE_CAPTURE"); path != "" {
			if shot != shotEnd {
				ext := filepath.Ext(path)
				path = fmt.Sprintf("%s-%.2f%s", strings.TrimSuffix(path, ext), shot, ext)
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, img); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
		}
		return img
	}
	var previous *image.RGBA
	var geometry, placements *wgpu.Buffer
	for _, shot := range []float64{22.4, 23.5, 26.56, 26.95, shotEnd} {
		img := drawScene(shot)
		if previous != nil {
			if bytes.Equal(previous.Pix, img.Pix) {
				t.Fatalf("GPU image did not move at shot %v", shot)
			}
			if r.vertices != geometry || r.placements != placements {
				t.Fatal("animation reallocates persistent geometry or placement buffers")
			}
		}
		previous, geometry, placements = img, r.vertices, r.placements
	}
	controller := newSceneController()
	controller.clock.elapsed = playbackDuration
	r.controller = controller
	drawScene(shotEnd)
	controller.selectCell(archiveCell{3, 14})
	settleController(t, controller)
	drawScene(36)
	controller.inspect(true)
	settleController(t, controller)
	drawScene(37)
	controller.inspect(false)
	settleController(t, controller)
	drawScene(38)
	controller.selectCell(archiveCell{maxLane, maxRow})
	settleController(t, controller)
	drawScene(39)
	controller.selectCell(archiveCell{minLane, minRow})
	settleController(t, controller)
	drawScene(40)
	if r.vertices != geometry || r.placements != placements {
		t.Fatal("interaction reallocates the persistent GPU buffers")
	}
}
