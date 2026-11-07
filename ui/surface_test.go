package ui

import (
	"math"
	"testing"
)

func TestPlanarSurfaceRoundTrip(t *testing.T) {
	s := NewPlanarSurface(Rect{X: 20, Y: 30, W: 200, H: 100})
	s.SetVersion(7)
	p, ok := s.SurfaceToScreen(UV{U: .25, V: .75})
	if !ok || p != (Point{X: 70, Y: 105}) {
		t.Fatalf("UV to screen = %+v, %v", p, ok)
	}
	hit, ok := s.ScreenToSurface(p)
	if !ok || absSurface(hit.UV.U-.25) > 1e-5 || absSurface(hit.UV.V-.75) > 1e-5 {
		t.Fatalf("screen to UV = %+v, %v", hit, ok)
	}
	if s.Version() != 7 {
		t.Fatalf("version = %d, want 7", s.Version())
	}
	if _, ok := s.ScreenToSurface(Point{X: 19, Y: 30}); ok {
		t.Fatal("point left of the panel hit")
	}
}

func TestProjectiveSurfaceRoundTrip(t *testing.T) {
	s := NewProjectiveSurface(Rect{X: 40, Y: 50, W: 300, H: 160},
		RotationY(28).Then(Perspective(800)))
	s.SetVersion(11)
	for _, uv := range []UV{{U: .1, V: .2}, {U: .5, V: .5}, {U: .9, V: .8}} {
		p, ok := s.SurfaceToScreen(uv)
		if !ok {
			t.Fatalf("UV %+v did not project", uv)
		}
		hit, ok := s.ScreenToSurface(p)
		if !ok || absSurface(hit.UV.U-uv.U) > 1e-4 || absSurface(hit.UV.V-uv.V) > 1e-4 || hit.Backface {
			t.Fatalf("UV %+v round trip = %+v, %v", uv, hit, ok)
		}
	}
	if s.Version() != 11 {
		t.Fatalf("version = %d, want 11", s.Version())
	}
}

func TestProjectiveSurfaceBackface(t *testing.T) {
	s := NewProjectiveSurface(Rect{W: 100, H: 100}, RotationY(180).Then(Perspective(800)))
	p, ok := s.SurfaceToScreen(UV{U: .5, V: .5})
	if ok {
		t.Fatalf("back face projected to %+v", p)
	}
	if _, ok := s.ScreenToSurface(Point{X: 50, Y: 50}); ok {
		t.Fatal("back face accepted a pointer hit")
	}
}

func TestProjectiveSurfaceBehindOrigin(t *testing.T) {
	s := NewProjectiveSurface(Rect{X: 1000, Y: 1000, W: 100, H: 100},
		RotationX(-60).Then(Perspective(600)))
	if !s.affine.Behind {
		t.Fatal("fixture must put the drawing origin behind the viewer")
	}
	for _, uv := range []UV{{U: .1, V: .2}, {U: .5, V: .5}, {U: .9, V: .8}} {
		p, ok := s.SurfaceToScreen(uv)
		if !ok {
			t.Fatalf("visible UV %+v did not project", uv)
		}
		hit, ok := s.ScreenToSurface(p)
		if !ok || absSurface(hit.UV.U-uv.U) > 1e-4 || absSurface(hit.UV.V-uv.V) > 1e-4 {
			t.Fatalf("UV %+v round trip = %+v, %v", uv, hit, ok)
		}
	}
}

func absSurface(v float32) float32 { return float32(math.Abs(float64(v))) }
