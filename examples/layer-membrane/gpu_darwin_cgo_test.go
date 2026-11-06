//go:build darwin && cgo

package main

import (
	"testing"
	"weak"

	"github.com/egoist/mygo/internal/gpu/metal"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/ui"
)

func TestMembraneMetalPassCompilesAndDraws(t *testing.T) {
	const width, height = 320, 220
	pix := make([]byte, 4*256*160)
	for y := 0; y < 160; y++ {
		for x := 0; x < 256; x++ {
			v := byte(50 + (x+y)%160)
			i := 4 * (y*256 + x)
			pix[i], pix[i+1], pix[i+2], pix[i+3] = v, v, v, 255
		}
	}
	source := scene.NewImageRGBA(256, 160, pix)
	canvas := scene.NewGPUCanvas(width, height, func(f scene.GPUFrame) error {
		return drawMembrane(ui.MetalFrame{
			Device: f.Device, CommandBuffer: f.CommandBuffer, Texture: f.Texture, Source: f.Source,
			Width: f.Width, Height: f.Height, SourceWidth: f.SourceWidth, SourceHeight: f.SourceHeight,
			Scale: f.Scale,
		}, 0, 0.14, 3.65, 0, 0, 1.1, 0.9, float32(width)/height)
	})
	canvas.GPU.Source = weak.Make(source)
	s := &scene.Scene{
		Width: width, Height: height,
		Canvases: []*scene.Image{canvas},
		Ops: []scene.Op{{
			Kind: scene.OpImage, Image: canvas,
			Rect: scene.Rect{X: 0, Y: 0, W: width, H: height},
			Src:  scene.Rect{X: 0, Y: 0, W: width, H: height},
		}},
	}
	pixels, err := metal.RenderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(pixels) != 4*width*height {
		t.Fatalf("rendered %d bytes, want %d", len(pixels), 4*width*height)
	}
	first := pixels[0]
	for i := 4; i < len(pixels); i += 4 {
		if pixels[i+2] != first {
			return
		}
	}
	t.Fatal("Metal membrane pass produced a flat image")
}
