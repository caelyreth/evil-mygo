//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"math"
	"testing"

	"github.com/caelyreth/evil-mygo/ui"
)

func TestClothHoistStaysPinned(t *testing.T) {
	s := &membraneSurface{}
	for _, amplitude := range []float32{0.25, 1, 1.6} {
		for _, waveTime := range []float32{0, 1.7, 4.2} {
			s.setState(camera{}, waveTime, amplitude)
			m := s.geometry()
			for row := range clothRows + 1 {
				p := m.vertices[row*(clothColumns+1)].position
				want := v3{-clothHalfW, panelLift + panelHalfH - 2*panelHalfH*float32(row)/clothRows, 0}
				if abs32(p.X-want.X) > 1e-6 || abs32(p.Y-want.Y) > 1e-6 || abs32(p.Z-want.Z) > 1e-6 {
					t.Fatalf("hoist row %d at t=%g, wind=%g: %+v, want %+v", row, waveTime, amplitude, p, want)
				}
			}
		}
	}
}

func TestClothStripLengthDoesNotStretchWithWind(t *testing.T) {
	s := &membraneSurface{amplitude: 1.6, waveTime: 2.3}
	m := s.geometry()
	want := float32(2 * clothHalfW / clothColumns)
	for row := range clothRows + 1 {
		for column := range clothColumns {
			i := row*(clothColumns+1) + column
			delta := sub3(m.vertices[i+1].position, m.vertices[i].position)
			length := float32(math.Sqrt(float64(delta.X*delta.X + delta.Z*delta.Z)))
			if abs32(length-want) > 1e-6 {
				t.Fatalf("row %d, column %d: strip step %g, want %g", row, column, length, want)
			}
		}
	}
}

func TestClothGeometryDoesNotDependOnViewportOrCamera(t *testing.T) {
	s := &membraneSurface{amplitude: 1, waveTime: 1.7}
	s.setViewport(1000, 700)
	m := s.geometry()
	version := m.version
	point, _ := m.point(ui.UV{U: 0.8, V: 0.4})
	s.setViewport(1600, 600)
	s.setState(camera{x: 2, y: 1, z: 3, yaw: 0.4}, s.waveTime, s.amplitude)
	next := s.geometry()
	if next != m || next.version != version {
		t.Fatal("camera or viewport change rebuilt the cloth")
	}
	if got, _ := next.point(ui.UV{U: 0.8, V: 0.4}); got != point {
		t.Fatalf("resizing stretched the flag: %+v, want %+v", got, point)
	}
	s.setState(s.cam, s.waveTime+0.3, s.amplitude)
	if s.geometry().version == version {
		t.Fatal("wind motion did not rebuild the cloth")
	}
}

func TestClothSideOnRayHitsActualTriangles(t *testing.T) {
	s := &membraneSurface{amplitude: 1, waveTime: 1.7}
	s.setViewport(1000, 700)
	p, _ := s.geometry().point(ui.UV{U: 0.573, V: 0.417})
	s.cam = camera{x: p.X - 2, y: p.Y, z: p.Z, yaw: float32(math.Pi / 2)}
	// No Z component: the old heightfield solver rejected this ray outright.
	hit, ok := s.intersect(v3{s.cam.x, s.cam.y, s.cam.z}, v3{X: 1})
	if !ok || hit.t > 2.001 {
		t.Fatalf("side-on intersection = %+v, %v", hit, ok)
	}
	if hit.u < 0 || hit.u > 1 || hit.v < 0 || hit.v > 1 {
		t.Fatalf("side-on intersection escaped the cloth: %+v", hit)
	}
}

func TestClothNearestFoldOccludesInputAndCaret(t *testing.T) {
	s := &membraneSurface{cam: camera{x: 0.25, y: panelLift, z: 3}, amplitude: 1}
	s.setViewport(1000, 700)
	m := s.geometry()
	// Fold the sheet over itself: two different UVs now cover the same ray.
	for i := range m.vertices {
		vertex := &m.vertices[i]
		vertex.position = v3{sin32(vertex.u * 2 * math.Pi), panelLift + panelHalfH - vertex.v*2*panelHalfH, vertex.u}
	}
	for i := range m.tiles {
		m.tiles[i] = clothBounds{lo: v3{-1, panelLift - panelHalfH, 0}, hi: v3{1, panelLift + panelHalfH, 1}}
	}
	hit, ok := s.ScreenToSurface(ui.Point{X: 500, Y: 350})
	if !ok || hit.UV.U < 0.4 || hit.UV.U > 0.5 || !hit.Backface {
		t.Fatalf("nearest folded hit = %+v, %v", hit, ok)
	}
	farUV := ui.UV{U: float32(math.Asin(0.25) / (2 * math.Pi)), V: 0.5}
	if screen, ok := s.SurfaceToScreen(farUV); ok {
		t.Fatalf("hidden front-face caret projected through the nearer fold: %+v", screen)
	}
}

func TestClothGroundOccludesTriangles(t *testing.T) {
	s := &membraneSurface{amplitude: 1, waveTime: 0.8}
	s.setViewport(1000, 700)
	p, _ := s.geometry().point(ui.UV{U: 0.5, V: 0.5})
	cam := v3{p.X, groundY - 1, p.Z + 2}
	rd := normalize3(sub3(p, cam))
	s.cam = camera{x: cam.X, y: cam.Y, z: cam.Z, pitch: float32(math.Asin(float64(rd.Y)))}
	if hit, ok := s.intersect(cam, rd); ok {
		t.Fatalf("ray crossed the opaque floor and reached %+v", hit)
	}
}
