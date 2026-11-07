//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"math"
	"slices"

	"github.com/egoist/mygo/ui"
)

// The file is planar: use MyGo's projective UV map, with the archive's
// existing raycast for occlusion. No second hit-test or input engine is needed.
type fileSurface struct {
	projection    *ui.ProjectiveSurface
	snapshot      sceneController
	pose          instance
	cell          archiveCell
	width, height float32
	aspect        float32
	enabled       bool
	back          bool
	version       uint64
}

func (s *fileSurface) update(c *sceneController, width, height float32, enabled bool) {
	var pose instance
	found := false
	for _, card := range c.shown {
		if card.cell == c.selected {
			pose, found = card.pose, true
			break
		}
	}
	enabled = enabled && found && c.hasShown && width > 0 && height > 0
	if enabled {
		normal := vec3{0, -pose.rotation[0], pose.rotation[1]}
		if s.back {
			normal = mul(normal, -1)
		}
		center := vec3{pose.position[0], pose.position[1] + 1.85, pose.position[2]}
		enabled = dot(normal, sub(c.shownCamera.eye, center)) > .18*c.shownCamera.distance
	}
	if s.projection != nil && s.snapshot.shownCamera == c.shownCamera && s.pose == pose && s.cell == c.selected && s.width == width && s.height == height &&
		s.aspect == c.shownAspect && s.enabled == enabled && slices.Equal(s.snapshot.shown, c.shown) {
		return
	}
	s.pose, s.cell = pose, c.selected
	s.width, s.height, s.aspect, s.enabled = width, height, c.shownAspect, enabled
	s.snapshot.shown = append(s.snapshot.shown[:0], c.shown...)
	s.snapshot.shownCamera, s.snapshot.shownAspect, s.snapshot.hasShown = c.shownCamera, c.shownAspect, c.hasShown
	s.version++
	if !enabled {
		return
	}
	cam := c.shownCamera
	sin, cos := pose.rotation[0], pose.rotation[1]
	z := float32(.07)
	if s.back {
		z = -.07
	}
	center := vec3{pose.position[0], pose.position[1] + 1.85*cos - z*sin, pose.position[2] + 1.85*sin + z*cos}
	delta := sub(center, cam.eye)
	direction := mul(cam.forward, -1)
	degrees := float32(180 / math.Pi)
	yaw := float32(math.Atan2(float64(direction.x), float64(direction.z))) * degrees
	elevation := float32(math.Asin(float64(min(max(direction.y, -1), 1)))) * degrees
	lean := float32(math.Atan2(float64(sin), float64(cos))) * degrees
	// MyGo's 3D transforms use screen Y-down and Z toward the viewer.
	// A centered parameter rectangle keeps the projection origin explicit.
	transform := ui.Scaling(5.0/dossierWidth, 3.7/dossierHeight)
	if s.back {
		// The reverse has its own outward normal and reads left-to-right when
		// seen from behind. Mirroring only the UVs would accept front-face input.
		transform = transform.Then(ui.RotationY(180))
	}
	transform = transform.Then(ui.RotationX(-lean)).
		Then(ui.Translation3D(delta.x, -delta.y, delta.z)).
		Then(ui.RotationY(-yaw)).Then(ui.RotationX(-elevation)).
		Then(ui.TranslationZ(cam.distance)).Then(ui.Perspective(cam.distance)).
		Then(ui.Scaling(width/(cam.span*s.aspect), height/cam.span)).
		Then(ui.Translation(width/2, height/2))
	if s.projection == nil {
		s.projection = ui.NewProjectiveSurface(ui.Rect{X: -dossierWidth / 2, Y: -dossierHeight / 2, W: dossierWidth, H: dossierHeight}, transform)
	} else {
		s.projection.SetTransform(transform)
	}
	s.projection.SetVersion(s.version)
}

func (s *fileSurface) Version() uint64 { return s.version }

func (s *fileSurface) ScreenToSurface(p ui.Point) (ui.SurfaceHit, bool) {
	if !s.enabled || s.projection == nil || p.X < 0 || p.Y < 0 || p.X >= s.width || p.Y >= s.height {
		return ui.SurfaceHit{}, false
	}
	hit, ok := s.projection.ScreenToSurface(p)
	if !ok || hit.Backface {
		return ui.SurfaceHit{}, false
	}
	cell, visible := s.snapshot.pick(p.X, p.Y, s.width, s.height)
	return hit, visible && cell == s.cell
}

func (s *fileSurface) SurfaceToScreen(uv ui.UV) (ui.Point, bool) {
	if !s.enabled || s.projection == nil {
		return ui.Point{}, false
	}
	p, ok := s.projection.SurfaceToScreen(uv)
	if !ok {
		return ui.Point{}, false
	}
	_, visible := s.ScreenToSurface(p)
	return p, visible
}

var _ ui.SurfaceMap = (*fileSurface)(nil)
