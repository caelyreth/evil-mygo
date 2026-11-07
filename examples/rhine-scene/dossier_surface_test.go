//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"math"
	"testing"

	"github.com/caelyreth/evil-mygo/ui"
)

func TestDossierUsesMyGoProjectionForRenderedPose(t *testing.T) {
	for _, size := range [][2]float32{{1440, 900}, {800, 1000}} {
		c := newSceneController()
		c.clock.elapsed = playbackDuration
		c.aspect = size[0] / size[1]
		c.inspect(true)
		settleController(t, c)
		c.snapshot(nil, c.aspect)
		surface := &fileSurface{}
		surface.update(c, size[0], size[1], true)
		for _, uv := range []ui.UV{{U: .25, V: .25}, {U: .5, V: .5}, {U: .75, V: .75}} {
			screen, ok := surface.SurfaceToScreen(uv)
			if !ok {
				t.Fatalf("native surface did not project visible UV %+v", uv)
			}
			pose, camera := surface.pose, c.shownCamera
			x, y, z := (uv.U-.5)*5, (1-uv.V)*3.7, float32(.07)
			sin, cos := pose.rotation[0], pose.rotation[1]
			world := vec3{x + pose.position[0], y*cos - z*sin + pose.position[1], y*sin + z*cos + pose.position[2]}
			right, up, forward, _ := lookAt(camera.eye, camera.aim)
			delta := sub(world, camera.eye)
			depth, tan := dot(delta, forward), camera.span/(2*camera.distance)
			want := ui.Point{
				X: (dot(delta, right)/(depth*tan*c.aspect) + 1) * size[0] / 2,
				Y: (1 - dot(delta, up)/(depth*tan)) * size[1] / 2,
			}
			if math.Abs(float64(screen.X-want.X)) > .1 || math.Abs(float64(screen.Y-want.Y)) > .1 {
				t.Fatalf("MyGo UV projection %+v differs from GPU pose %+v", screen, want)
			}
			hit, ok := surface.ScreenToSurface(screen)
			if !ok || math.Abs(float64(hit.UV.U-uv.U)) > .0005 || math.Abs(float64(hit.UV.V-uv.V)) > .0005 {
				t.Fatalf("native surface round trip: %+v -> %+v (%v)", uv, hit, ok)
			}
		}
		version := surface.Version()
		surface.update(c, size[0], size[1], true)
		if surface.Version() != version {
			t.Fatal("unchanged surface invalidated the host's geometry binding")
		}
		surface.update(c, size[0], size[1], false)
		if _, ok := surface.SurfaceToScreen(ui.UV{U: .5, V: .5}); ok {
			t.Fatal("closed file still exposes native controls")
		}
	}
}

func TestReverseSurfaceUsesOppositePlaneAndIndependentUVs(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed, c.aspect = playbackDuration, 16.0/9
	c.inspect(true)
	settleController(t, c)
	c.turnFile()
	settleController(t, c)
	c.snapshot(nil, c.aspect)
	front, back := &fileSurface{}, &fileSurface{back: true}
	front.update(c, 1440, 810, true)
	back.update(c, 1440, 810, true)
	if _, ok := front.SurfaceToScreen(ui.UV{U: .5, V: .5}); ok {
		t.Fatal("front input map accepts input through the back of the file")
	}
	for _, uv := range []ui.UV{{U: .25, V: .3}, {U: .5, V: .5}, {U: .75, V: .7}} {
		screen, ok := back.SurfaceToScreen(uv)
		if !ok {
			t.Fatalf("reverse face did not project %+v", uv)
		}
		x, y, z := (.5-uv.U)*5, (1-uv.V)*3.7, float32(-.07)
		pose := back.pose
		sin, cos := pose.rotation[0], pose.rotation[1]
		world := vec3{x + pose.position[0], y*cos - z*sin + pose.position[1], y*sin + z*cos + pose.position[2]}
		right, up, forward, _ := lookAt(c.shownCamera.eye, c.shownCamera.aim)
		delta := sub(world, c.shownCamera.eye)
		depth, tan := dot(delta, forward), c.shownCamera.span/(2*c.shownCamera.distance)
		want := ui.Point{X: (dot(delta, right)/(depth*tan*c.aspect) + 1) * 720,
			Y: (1 - dot(delta, up)/(depth*tan)) * 405}
		if math.Abs(float64(screen.X-want.X)) > .1 || math.Abs(float64(screen.Y-want.Y)) > .1 {
			t.Fatal("reverse UVs did not match the mirrored back-plane texture")
		}
		hit, ok := back.ScreenToSurface(screen)
		if !ok || math.Abs(float64(hit.UV.U-uv.U)) > .0005 || math.Abs(float64(hit.UV.V-uv.V)) > .0005 {
			t.Fatal("reverse surface input did not round-trip independently")
		}
	}
}

func TestReverseDossierKeepsCustodySeparateFromAnalysis(t *testing.T) {
	cell := archiveCell{2, 12}
	front, back := newFileDossier(ui.LightTheme(), cell), newBackDossier(ui.LightTheme(), cell)
	t.Cleanup(front.layer.Close)
	t.Cleanup(back.layer.Close)
	front.record().scans, front.record().draft = 7, "Front laboratory note"
	back.custody().transfers, back.custody().draft, back.custody().seal = 3, "Reverse handling note", true
	back.selectCell(archiveCell{3, 12})
	back.custody().hold = true
	back.selectCell(cell)
	if front.layer == back.layer || front.record().scans != 7 || front.record().draft != "Front laboratory note" {
		t.Fatal("reverse UI changes leaked into the front analysis engine or record")
	}
	if back.custody().transfers != 3 || back.custody().draft != "Reverse handling note" || !back.custody().seal || back.custody().hold {
		t.Fatal("reverse custody state was not retained per file")
	}
}
