package ui

import "github.com/egoist/mygo/internal/scene"

// UV is a point in a surface's parameter space. U runs across the surface
// and V runs down it, both from 0 to 1.
type UV struct {
	U, V float32
}

// SurfaceHit is a pointer hit on a presented surface.
type SurfaceHit struct {
	UV       UV
	Depth    float32
	Backface bool
}

// SurfaceMap maps between window points and the parameter space of a
// presented surface. Implementations for cloth, heightfields, meshes, or
// other deformable surfaces belong to the package that owns that surface.
//
// Version identifies the geometry snapshot the mapping was made from.
// Callers comparing a pointer hit with a rendered layer should use the
// same version that rendered that layer; a stale mapping can put a click
// beside the pixels the user saw. A map may be rebuilt each frame, or may
// keep a stable version while its surface is unchanged.
type SurfaceMap interface {
	Version() uint64
	ScreenToSurface(Point) (SurfaceHit, bool)
	SurfaceToScreen(UV) (Point, bool)
}

// PlanarSurface maps a rectangular panel with no transform. It is the
// simple case used by ordinary native panels and cards.
type PlanarSurface struct {
	rect    Rect
	version uint64
}

// NewPlanarSurface returns a map for rect. The rect is in window DIPs.
func NewPlanarSurface(rect Rect) *PlanarSurface {
	return &PlanarSurface{rect: rect}
}

// SetRect changes the panel's screen rectangle.
func (s *PlanarSurface) SetRect(rect Rect) { s.rect = rect }

// SetVersion records the geometry version this map describes.
func (s *PlanarSurface) SetVersion(v uint64) { s.version = v }

func (s *PlanarSurface) Version() uint64 { return s.version }

func (s *PlanarSurface) ScreenToSurface(p Point) (SurfaceHit, bool) {
	if s.rect.W <= 0 || s.rect.H <= 0 ||
		p.X < s.rect.X || p.X >= s.rect.X+s.rect.W ||
		p.Y < s.rect.Y || p.Y >= s.rect.Y+s.rect.H {
		return SurfaceHit{}, false
	}
	return SurfaceHit{UV: UV{
		U: (p.X - s.rect.X) / s.rect.W,
		V: (p.Y - s.rect.Y) / s.rect.H,
	}}, true
}

func (s *PlanarSurface) SurfaceToScreen(uv UV) (Point, bool) {
	if s.rect.W <= 0 || s.rect.H <= 0 || uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 {
		return Point{}, false
	}
	return Point{
		X: s.rect.X + uv.U*s.rect.W,
		Y: s.rect.Y + uv.V*s.rect.H,
	}, true
}

// ProjectiveSurface maps a rectangular surface through a Transform,
// including a RotateY/Perspective pair. The inverse is used for pointer
// input, so a surface that turns away from the viewer reports no hit.
type ProjectiveSurface struct {
	rect      Rect
	transform Transform
	affine    scene.Affine
	inverse   scene.Affine
	invertOK  bool
	facesAway bool
	version   uint64
}

// NewProjectiveSurface returns a map for rect transformed by t. The
// transform is around the center of rect, as Element.Transform is around
// an element's center by default.
func NewProjectiveSurface(rect Rect, t Transform) *ProjectiveSurface {
	s := &ProjectiveSurface{rect: rect, transform: t}
	s.rebuild()
	return s
}

// SetRect changes the panel's screen rectangle and recomputes the map.
func (s *ProjectiveSurface) SetRect(rect Rect) {
	s.rect = rect
	s.rebuild()
}

// SetTransform changes the surface transform and recomputes the map.
func (s *ProjectiveSurface) SetTransform(t Transform) {
	s.transform = t
	s.rebuild()
}

// SetVersion records the geometry version this map describes.
func (s *ProjectiveSurface) SetVersion(v uint64) { s.version = v }

func (s *ProjectiveSurface) Version() uint64 { return s.version }

func (s *ProjectiveSurface) rebuild() {
	// Transform acts around the element's center by default.
	cx, cy := s.rect.X+s.rect.W/2, s.rect.Y+s.rect.H/2
	s.affine = scene.Translation(-cx, -cy).Mul(s.transform.flat()).Mul(scene.Translation(cx, cy))
	s.inverse, s.invertOK = s.affine.Inverse()
	s.facesAway = s.transform.facesAway()
}

func (s *ProjectiveSurface) ScreenToSurface(p Point) (SurfaceHit, bool) {
	if s.facesAway || !s.invertOK || s.rect.W <= 0 || s.rect.H <= 0 {
		return SurfaceHit{}, false
	}
	x, y := s.inverse.Point(p.X, p.Y)
	u := (x - s.rect.X) / s.rect.W
	v := (y - s.rect.Y) / s.rect.H
	if u < 0 || u > 1 || v < 0 || v > 1 {
		return SurfaceHit{}, false
	}
	return SurfaceHit{UV: UV{U: u, V: v}, Backface: s.facesAway}, true
}

func (s *ProjectiveSurface) SurfaceToScreen(uv UV) (Point, bool) {
	if !s.invertOK || s.facesAway || s.rect.W <= 0 || s.rect.H <= 0 || uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 {
		return Point{}, false
	}
	x, y := s.rect.X+uv.U*s.rect.W, s.rect.Y+uv.V*s.rect.H
	if s.affine.W(x, y) <= 0 {
		return Point{}, false
	}
	x, y = s.affine.Point(x, y)
	return Point{X: x, Y: y}, true
}
