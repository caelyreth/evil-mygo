package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestCameraInput(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	z := a.cam.z
	tt.KeyDown(ui.KeyW)
	tt.Advance(100 * time.Millisecond)
	tt.KeyUp(ui.KeyW)
	if a.cam.z >= z {
		t.Fatalf("W moved camera z from %v to %v", z, a.cam.z)
	}

	yaw := a.cam.yaw
	tt.Press(420, 420)
	tt.Move(500, 450)
	tt.Release(500, 450)
	if a.cam.yaw == yaw || a.drag {
		t.Fatalf("mouse look left yaw %v at %v, dragging %v", yaw, a.cam.yaw, a.drag)
	}
}

func TestPanelControls(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	if !tt.FramePending() {
		t.Fatal("animated cloth does not request frames")
	}
	if err := tt.Click("Cloth motion"); err != nil {
		t.Fatal(err)
	}
	if a.animate || tt.FramePending() {
		t.Fatal("paused cloth still requests frames")
	}
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if a.cam.z != 3.8 || a.cam.x != 0 || a.cam.pitch != 0 {
		t.Fatalf("Reset view left camera at %+v", a.cam)
	}
}
