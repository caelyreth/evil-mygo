//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"testing"
	"time"

	"github.com/caelyreth/evil-mygo/ui"
)

func TestNativeHUDAndKeyboardUseController(t *testing.T) {
	controller := newSceneController()
	controller.clock.elapsed = playbackDuration
	controller.snapshot(nil, 960.0/700)
	renderer := &sceneRenderer{controller: controller, draws: 1}
	app := newArchiveApp(ui.NewWebGPUCanvas(1, 1, nil), renderer)
	t.Cleanup(app.close)
	tt := ui.NewTester(app.view, 960, 700)
	tt.Advance(3 * time.Second) // The selected file's dedicated cover also settles.
	if !controller.interactive || controller.inspecting || tt.FramePending() {
		t.Fatal("entrance did not hand off to selection without opening")
	}
	tt.Key(0, ui.KeyR)
	tt.Key(0, ui.KeySpace)
	if !controller.interactive || controller.clock.elapsed != playbackDuration || controller.moving() || controller.inspecting {
		t.Fatal("removed playback shortcuts still control the scene")
	}
	tt.Click("Next file")
	tt.Advance(10 * time.Second)
	if controller.selected != (archiveCell{2, 13}) || tt.FramePending() {
		t.Fatal("native navigation is not connected to selection")
	}
	tt.Click("Inspect file")
	tt.Advance(10 * time.Second)
	if !controller.inspecting || controller.detail.value != 1 || tt.FramePending() {
		t.Fatal("native Inspect does not settle")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(10 * time.Second)
	if controller.inspecting {
		t.Fatal("Escape did not return")
	}
	tt.Key(0, ui.KeyRight)
	tt.Advance(10 * time.Second)
	if controller.selected != (archiveCell{3, 13}) {
		t.Fatal("arrow navigation bypasses the shared controller")
	}
}

func TestScenePointerDoubleClickAndCancel(t *testing.T) {
	controller := newSceneController()
	controller.clock.elapsed = playbackDuration
	controller.snapshot(nil, 16.0/9)
	app := newArchiveApp(ui.NewWebGPUCanvas(1, 1, nil), &sceneRenderer{controller: controller, draws: 1})
	t.Cleanup(app.close)
	app.width, app.height = 1600, 900
	// Project the raised card's center using the same camera as the pick ray.
	pose := controller.card(controller.selected).position
	camera := controller.shownCamera
	right, up, forward, _ := lookAt(camera.eye, camera.aim)
	delta := sub(vec3{pose[0], pose[1] + 1.85, pose[2]}, camera.eye)
	depth := dot(delta, forward)
	tan := camera.span / (2 * camera.distance)
	x := (dot(delta, right)/(depth*tan*(16.0/9)) + 1) * 800
	y := (1 - dot(delta, up)/(depth*tan)) * 450
	app.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y, Clicks: 1})
	app.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: y, Canceled: true})
	if controller.interactive {
		t.Fatal("a canceled press selected a card")
	}
	app.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y, Clicks: 1})
	app.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: y, Clicks: 1})
	controller.inspect(false)
	app.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: x, Y: y, Clicks: 2})
	app.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: x, Y: y, Clicks: 2})
	if !controller.interactive || !controller.inspecting {
		t.Fatal("double click did not inspect the picked card")
	}
}

func TestNativeNavigationObeysArchiveCaps(t *testing.T) {
	controller := newSceneController()
	controller.aspect = 960.0 / 700
	controller.selectCell(archiveCell{maxLane, maxRow})
	controller.inspect(true)
	settleController(t, controller)
	controller.snapshot(nil, controller.aspect)
	controller.dirty = false
	app := newArchiveApp(ui.NewWebGPUCanvas(1, 1, nil), &sceneRenderer{controller: controller, draws: 1})
	t.Cleanup(app.close)
	tt := ui.NewTester(app.view, 960, 700)
	tt.Click("Next file")
	tt.Click("Next column")
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyDown)
	app.input(ui.InputEvent{Kind: ui.InputScroll, DY: 120})
	app.input(ui.InputEvent{Kind: ui.InputScroll, DX: 120})
	if controller.selected != (archiveCell{maxLane, maxRow}) || !controller.inspecting || controller.active || controller.dirty {
		t.Fatal("button, keyboard or wheel navigation bypassed the archive cap")
	}
}

func TestOutsideExaminedCardClickOnlyReturns(t *testing.T) {
	for _, test := range []struct {
		name  string
		cards []shownCard
	}{
		{"empty scene", nil},
		{"another file", []shownCard{{archiveCell{3, 12}, instance{rotation: [4]float32{0, 1}}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			controller := newSceneController()
			controller.inspect(true)
			settleController(t, controller)
			selected := controller.selected
			controller.shown, controller.hasShown, controller.shownAspect = test.cards, true, 16.0/9
			controller.shownCamera = sceneCamera{eye: vec3{0, 1.8, 10}, aim: vec3{0, 1.8, 0},
				forward: vec3{0, 0, -1}, span: 8, distance: 10}
			app := newArchiveApp(ui.NewWebGPUCanvas(1, 1, nil), &sceneRenderer{controller: controller, draws: 1})
			t.Cleanup(app.close)
			app.width, app.height = 1600, 900
			app.hasClick, app.lastClickX, app.lastClickY = true, 800, 450
			app.input(ui.InputEvent{Kind: ui.InputPointerDown, Button: 0, X: 800, Y: 450, Clicks: 2})
			app.input(ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, X: 800, Y: 450, Clicks: 2})
			if controller.inspecting || controller.selected != selected || app.hasClick {
				t.Fatal("an outside click selected another file or reopened the current file")
			}
		})
	}
}
