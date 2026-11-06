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
	if err := tt.Click("Animate waves"); err != nil {
		t.Fatal(err)
	}
	if a.animate || tt.FramePending() {
		t.Fatal("paused Metal layer still requests frames")
	}
}
