//go:build darwin

package webgpu

import (
	"testing"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkRenderSprites draws gputest's sprite-heavy game frame into a
// reusable WebGPU texture and waits for the device after each submission.
func BenchmarkRenderSprites(b *testing.B) {
	benchRender(b, gputest.SpriteScene(1920, 1080, 3, 3, 300))
}

// BenchmarkRenderSpritesZoomedOut draws the same game with about 7,000
// sprites.
func BenchmarkRenderSpritesZoomedOut(b *testing.B) {
	benchRender(b, gputest.SpriteScene(1920, 1080, 1.5, 3, 1000))
}

func BenchmarkRenderEmpty(b *testing.B) {
	benchRender(b, &scene.Scene{Width: 1920, Height: 1080, Clear: scene.Color{A: 255}})
}

func BenchmarkRenderBoxes(b *testing.B) {
	benchRender(b, gputest.BoxesScene(1280, 800, 600, false))
}

func BenchmarkRenderTranslatedBoxes(b *testing.B) {
	benchRender(b, gputest.BoxesScene(1280, 800, 600, true))
}

func benchRender(b *testing.B, s *scene.Scene) {
	g, err := Open()
	if err != nil {
		b.Skip(err)
	}
	defer g.Release()
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Label:         "WebGPU benchmark target",
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size:          wgpu.Extent3D{Width: uint32(s.Width), Height: uint32(s.Height), DepthOrArrayLayers: 1},
		Format:        wgpu.TextureFormatRGBA8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		b.Fatal(err)
	}
	defer target.Release()
	view, err := target.CreateView(nil)
	if err != nil {
		b.Fatal(err)
	}
	defer view.Release()
	r, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Release()
	b.ResetTimer()
	for b.Loop() {
		encoder, err := g.Device().CreateCommandEncoder(nil)
		if err != nil {
			b.Fatal(err)
		}
		if err := r.Render(s, encoder, view); err != nil {
			encoder.Release()
			b.Fatal(err)
		}
		command, err := encoder.Finish(nil)
		if err != nil {
			encoder.Release()
			b.Fatal(err)
		}
		encoder.Release()
		if _, err := g.Queue().Submit(command); err != nil {
			command.Release()
			b.Fatal(err)
		}
		command.Release()
		r.FinishFrame()
		g.Device().Poll(true)
	}
}

// BenchmarkRenderFillRate draws large layers, a border ring and stacked
// shadows at a Retina-sized target.
func BenchmarkRenderFillRate(b *testing.B) {
	const w, h = 3456, 2000
	fill := func(r scene.Rect, c scene.Color) scene.Op {
		return scene.Op{Kind: scene.OpFill, Rect: r, Color: c}
	}
	background := fill(scene.Rect{W: w, H: h}, scene.Color{R: 20, G: 22, B: 28, A: 255})
	var layers []scene.Op
	for i := range 6 {
		d := float32(i)
		layers = append(layers, fill(scene.Rect{X: d, Y: d, W: w - 2*d, H: h - 2*d}, scene.Color{R: byte(20 + i), G: 22, B: 28, A: 255}))
	}
	panel := scene.Rect{X: 576, Y: 48, W: 2800, H: 1856}
	radii := scene.Uniform(24)
	card := fill(panel, scene.Color{R: 30, G: 32, B: 38, A: 255})
	card.Radii = radii
	card.Continuous = true
	ring := fill(panel, scene.Color{})
	ring.Radii = radii
	ring.Continuous = true
	ring.Border = scene.Uniform(2)
	ring.BorderColor = scene.Color{A: 100}
	shadows := []scene.Op{background, fill(scene.Rect{W: w, H: h}, scene.Color{A: 100})}
	for _, blur := range []float32{6, 16, 32, 64} {
		shadows = append(shadows, scene.Op{
			Kind: scene.OpShadow, Rect: panel, Radii: radii, Cast: panel,
			CastRadii: radii, Blur: blur, Color: scene.Color{A: 60},
		})
	}
	shadows = append(shadows, card)
	for _, tc := range []struct {
		name string
		ops  []scene.Op
	}{
		{"nested_layers", layers},
		{"border_ring", []scene.Op{background, card, ring}},
		{"shadow_panel", shadows},
	} {
		b.Run(tc.name, func(b *testing.B) {
			benchRender(b, &scene.Scene{Width: w, Height: h, Clear: scene.Color{A: 255}, Ops: tc.ops})
		})
	}
}
