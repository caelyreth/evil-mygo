//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSceneSizesOnFirstPaint(t *testing.T) {
	canvas := ui.NewWebGPUCanvas(1, 1, nil)
	view := sceneView(canvas, nil)
	builds := 0
	tt := ui.NewTester(func(c *ui.Context) {
		builds++
		view(c)
	}, 640, 400)
	check := func(width, height int) {
		t.Helper()
		if w, h := canvas.Size(); w != width || h != height {
			t.Fatalf("canvas size = %dx%d, want %dx%d in the same frame", w, h, width, height)
		}
		if tt.FramePending() {
			t.Fatal("static scene keeps requesting frames")
		}
	}
	check(640, 400)
	if builds != 1 {
		t.Fatalf("initial layout needed %d builds, want one", builds)
	}
	tt.SetScale(1.5)
	check(960, 600)
	tt.SetSize(777, 513)
	check(1166, 770)
	tt.SetScale(2)
	check(1554, 1026)
	if builds != 4 {
		t.Fatalf("startup and three changes needed %d builds, want four", builds)
	}
}

func TestSceneEntrancePlaysOnceThenSleeps(t *testing.T) {
	canvas := ui.NewWebGPUCanvas(1, 1, nil)
	renderer := &sceneRenderer{}
	tt := ui.NewTester(sceneView(canvas, renderer), 640, 400)
	tt.Advance(time.Second)
	if renderer.clock.elapsed != 0 || !tt.FramePending() {
		t.Fatal("playback skipped the opening before its first GPU frame")
	}
	renderer.draws = 1
	tt.Advance(time.Second)
	if renderer.clock.elapsed <= .9 || renderer.clock.elapsed >= 1.1 {
		t.Fatalf("playback did not advance without input: %v", renderer.clock.elapsed)
	}
	tt.SetWindowHidden(true)
	paused := renderer.clock.elapsed
	tt.Advance(time.Minute)
	if renderer.clock.elapsed != paused || tt.FramePending() {
		t.Fatal("playback did not sleep while hidden")
	}
	tt.SetWindowHidden(false)
	if renderer.clock.elapsed != paused || !tt.FramePending() {
		t.Fatal("playback did not resume where it paused")
	}
	tt.Advance(15 * time.Second)
	if renderer.clock.elapsed != playbackDuration || tt.FramePending() {
		t.Fatalf("playback did not hold its final frame: elapsed %v, pending %v", renderer.clock.elapsed, tt.FramePending())
	}
}
