//go:build darwin

package webgpu

import (
	"testing"

	"github.com/caelyreth/evil-mygo/internal/gpu/gputest"
	"github.com/caelyreth/evil-mygo/internal/scene"
)

func TestRendererUpstreamGeometry(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	for _, tc := range []struct {
		name  string
		scene *scene.Scene
	}{
		{"behind-origin", gputest.BehindOriginScene()},
		{"corner-clips", gputest.CornerClipScene()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pixels, err := RenderOffscreen(tc.scene)
			if err != nil {
				t.Fatal(err)
			}
			gputest.Compare(t, "webgpu-"+tc.name, pixels, tc.scene.Width*4, tc.scene)
		})
	}
}

func TestEffectBackdropBehindOrigin(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	// The card is visible while its local origin lies past its horizon.
	// Sampling its backdrop must use the same signed divisor as drawing it.
	s := gputest.BehindOriginScene()
	card := s.Ops[1]
	red := scene.Color{R: 220, G: 40, B: 40, A: 255}
	s.Ops = []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{X: 80, Y: 280, W: 160, H: 180}, Color: red},
	}
	before, err := RenderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Effects = []scene.EffectOp{{Effect: &scene.Effect{
		Name: "behind-origin backdrop", Backdrop: true,
		WGSL: `
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	return vec4<f32>(effect_sample_backdrop(e, p), 1.0);
}`,
	}}}
	s.Ops = append(s.Ops, scene.Op{
		Kind: scene.OpEffect, Rect: card.Rect, Transform: card.Transform,
	})
	after, err := RenderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	i := (370*s.Width + 160) * 4
	for channel, want := range before[i : i+4] {
		if got := after[i+channel]; got != want {
			t.Fatalf("effect changed backdrop channel %d: got %d, want %d", channel, got, want)
		}
	}
	// A constant-color effect must also draw the visible card. Otherwise a
	// missing effect would pass the backdrop comparison without being tested.
	s.Effects[0].Effect = &scene.Effect{
		Name: "behind-origin solid",
		WGSL: `
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	return vec4<f32>(0.0, 1.0, 0.0, 1.0);
}`,
	}
	shown, err := RenderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	if p := shown[i : i+4]; p[0] != 0 || p[1] != 255 || p[2] != 0 || p[3] != 255 {
		t.Fatalf("effect did not draw the visible card: %v", p)
	}
}
