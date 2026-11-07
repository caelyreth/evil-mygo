//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package ui

import (
	"testing"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/internal/scene"
)

// BenchmarkStressFrameWebGPU runs the stress scenes through a reusable
// WebGPU device and target. It waits for each submission, so ns/op includes
// CPU encoding and GPU execution; it is not a display-latency measurement.
func BenchmarkStressFrameWebGPU(b *testing.B) {
	g, err := webgpu.Open()
	if err != nil {
		b.Skip(err)
	}
	defer g.Release()
	for _, sc := range stressScenes {
		b.Run(sc.name, func(b *testing.B) {
			target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
				Label:         "MyGo stress target",
				Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
				Size:          wgpu.Extent3D{Width: stressW * stressScale, Height: stressH * stressScale, DepthOrArrayLayers: 1},
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
			renderer, err := webgpu.NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
			if err != nil {
				b.Fatal(err)
			}
			defer renderer.Release()
			var renderErr error
			f := newStressFrames(sc.view, func(s *scene.Scene) {
				if renderErr != nil {
					return
				}
				encoder, err := g.Device().CreateCommandEncoder(nil)
				if err != nil {
					renderErr = err
					return
				}
				if err := renderer.Render(s, encoder, view); err != nil {
					encoder.Release()
					renderErr = err
					return
				}
				command, err := encoder.Finish(nil)
				if err != nil {
					encoder.Release()
					renderErr = err
					return
				}
				encoder.Release()
				if _, err := g.Queue().Submit(command); err != nil {
					command.Release()
					renderErr = err
					return
				}
				command.Release()
				renderer.FinishFrame()
				g.Device().Poll(true)
			})
			b.ResetTimer()
			for b.Loop() {
				f.next()
			}
			if renderErr != nil {
				b.Fatal(renderErr)
			}
		})
	}
}
