package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestNativeControlsAroundMetalCanvas(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1120, 720)
	if err := tt.Click("Canvas title"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("An edited canvas")
	if a.title != "An edited canvas" {
		t.Fatalf("title = %q, want edited text", a.title)
	}
	if err := tt.Click("Animate Metal"); err != nil {
		t.Fatal(err)
	}
	if a.animate {
		t.Fatal("Animate Metal did not pause the canvas")
	}
	if tt.FramePending() {
		t.Fatal("paused canvas still asks for frames")
	}
	if err := tt.Click("Animate Metal"); err != nil {
		t.Fatal(err)
	}
	if !a.animate || !tt.FramePending() {
		t.Fatal("Animate Metal did not resume frames")
	}
}
