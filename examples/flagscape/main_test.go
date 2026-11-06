package main

import (
	"math"
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
	a.cam.z = 0.15
	tt.KeyDown(ui.KeyW)
	tt.Advance(200 * time.Millisecond)
	tt.KeyUp(ui.KeyW)
	if a.cam.z >= 0 {
		t.Fatalf("camera is still restricted in front of the flag: z = %v", a.cam.z)
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
	if err := tt.Click("Reset view"); err != nil {
		t.Fatal(err)
	}
	if a.cam.z != 3.8 || a.cam.x != 0 || a.cam.pitch != 0 {
		t.Fatalf("Reset view left camera at %+v", a.cam)
	}
}

func TestFlagActionUsesUVHit(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	if a.hitBox.W <= 0 || a.viewBox.W <= 0 {
		t.Fatalf("action/view boxes are not laid out: action=%+v view=%+v", a.hitBox, a.viewBox)
	}
	u := (a.hitBox.X - a.viewBox.X + a.hitBox.W/2) / a.viewBox.W
	v := (a.hitBox.Y - a.viewBox.Y + a.hitBox.H/2) / a.viewBox.H
	x, y, ok := panelScreenPoint(a, u, v, a.viewBox.W, a.viewBox.H)
	if !ok {
		t.Fatal("action center does not project onto the flag")
	}
	tt.Press(x, y)
	tt.Release(x, y)
	if a.actions != 1 {
		t.Fatalf("clicking the UV-mapped action counted %d actions, want 1", a.actions)
	}
}

func panelScreenPoint(a *app, u, v, w, h float32) (float32, float32, bool) {
	halfH := float32(0.68)
	halfW := halfH * w / h
	x := -halfW + u*2*halfW
	y := panelLift + halfH - v*2*halfH
	p := v3{x, y, panelThickness}
	cam := v3{a.cam.x, a.cam.y, a.cam.z}
	fwd := v3{
		float32(math.Sin(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
		float32(math.Sin(float64(a.cam.pitch))),
		-float32(math.Cos(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
	}
	right := normalize3(cross3(fwd, v3{0, 1, 0}))
	up := normalize3(cross3(right, fwd))
	rel := add3(p, scale3(cam, -1))
	z := dot3(rel, fwd)
	if z <= 0 {
		return 0, 0, false
	}
	nx := dot3(rel, right) / (z * 0.74 * (w / h))
	ny := dot3(rel, up) / (z * 0.74)
	return (nx + 1) * w / 2, (1 - ny) * h / 2, true
}

func dot3(a, b v3) float32 { return a.x*b.x + a.y*b.y + a.z*b.z }
