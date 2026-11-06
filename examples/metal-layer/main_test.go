package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLiveNativeLayerControls(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1120, 720)
	if err := tt.Click("Display name"); err != nil {
		t.Fatal(err)
	}
	active, _ := tt.TextInputActive()
	caret, ok := tt.TextCaret()
	if !active || !ok || caret.W <= 0 || caret.H <= 0 {
		t.Fatalf("tilted field has no input-method caret: active=%v caret=%+v ok=%v", active, caret, ok)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("A real native field")
	if a.name != "A real native field" {
		t.Fatalf("name = %q, want edited text", a.name)
	}
	if err := tt.Click("Acknowledge profile"); err != nil {
		t.Fatal(err)
	}
	if !a.done || !tt.HasText("Acknowledged") {
		t.Fatal("the native button did not update the source tree")
	}
	scroll, ok := tt.Find("Recent activity list")
	if !ok {
		t.Fatal("the native scroll area is not visible")
	}
	tt.Scroll(scroll.X+scroll.W/2, scroll.Y+scroll.H/2, 0, 20)
	tt.Frame()
	if a.scroll.Y <= 0 {
		t.Fatalf("scroll offset after wheel = %+v, want a scrolled content area", a.scroll)
	}
	before, ok := tt.Access("Display name")
	if !ok {
		t.Fatal("the native source field has no accessibility node")
	}
	a.tilt = 0.85
	tt.Frame()
	after, ok := tt.Access("Display name")
	if !ok || after.Bounds == before.Bounds {
		t.Fatalf("field accessibility bounds before %+v, after %+v", before.Bounds, after.Bounds)
	}
	if err := tt.Click("Display name"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("Focus follows the plane")
	if a.name != "Focus follows the plane" {
		t.Fatalf("name after the tilted edit = %q", a.name)
	}
	if err := tt.Click("Acknowledge profile"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Animate waves"); err != nil {
		t.Fatal(err)
	}
	if a.animate || tt.FramePending() {
		t.Fatal("paused Metal layer still requests frames")
	}
}
