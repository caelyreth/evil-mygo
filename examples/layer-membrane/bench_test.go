//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func BenchmarkMembraneScreenToSurface(b *testing.B) {
	s := &membraneSurface{
		cam:       camera{z: 3.65, y: 0.14},
		amplitude: 1,
		waveTime:  1.7,
	}
	s.setViewport(1120, 852)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, ok := s.ScreenToSurface(ui.Point{X: 684, Y: 508}); !ok {
			b.Fatal("benchmark ray missed")
		}
	}
}

func BenchmarkMembraneSurfaceToScreen(b *testing.B) {
	s := &membraneSurface{
		cam:       camera{z: 3.65, y: 0.14},
		amplitude: 1,
		waveTime:  1.7,
	}
	s.setViewport(1120, 852)
	uv := ui.UV{U: .55, V: .57}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, ok := s.SurfaceToScreen(uv); !ok {
			b.Fatal("benchmark point did not project")
		}
	}
}
