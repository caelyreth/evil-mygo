package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLayerSurfaceInput(t *testing.T) {
	a := newApp()
	ui.NewTester(a.view, 1240, 760)
	info, ok := a.layer.Access("Press layer")
	if !ok {
		t.Fatal("layer button has no accessibility node")
	}
	u := (info.Bounds.X + info.Bounds.W/2) / layerWidth
	v := (info.Bounds.Y + info.Bounds.H/2) / layerHeight
	p, ok := a.surface.SurfaceToScreen(ui.UV{U: u, V: v})
	if !ok {
		t.Fatal("button center does not project")
	}
	a.layer.HandleSurfaceInput(a.surface, ui.InputEvent{Kind: ui.InputPointerDown, X: p.X, Y: p.Y, Button: 0})
	a.layer.HandleSurfaceInput(a.surface, ui.InputEvent{Kind: ui.InputPointerUp, X: p.X, Y: p.Y, Button: 0})
	a.layer.Render()
	if a.clicks != 1 {
		t.Fatalf("layer click count = %d, want 1", a.clicks)
	}
}
