package main

import (
	"math"
	"testing"
	"time"
)

func settleController(t *testing.T, c *sceneController) {
	t.Helper()
	for frame := 0; frame < 1200 && c.active; frame++ {
		c.time += 1.0 / 60
		c.step(1.0 / 60)
	}
	if c.active {
		t.Fatal("interactive controller never settles")
	}
}

func TestInteractiveSelectionAndReturn(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	_, before, _ := c.snapshot(nil, 16.0/9)
	previous := c.card(c.selected)
	c.selectCell(archiveCell{2, 13})
	if c.current != before || c.card(archiveCell{2, 12}).position != previous.position {
		t.Fatal("interrupting the entrance jumps the camera or outgoing card")
	}
	settleController(t, c)
	if c.selected != (archiveCell{2, 13}) || c.inspecting || math.Abs(c.cards[c.selected].lift.value-.4) > .00001 {
		t.Fatal("selection did not settle at preview height")
	}
	if c.cards[archiveCell{2, 12}].lift.value != 0 {
		t.Fatal("the outgoing card never returned")
	}
	c.inspect(true)
	settleController(t, c)
	if c.cards[c.selected].lift.value != 4.05 || c.detail.value != 1 {
		t.Fatal("inspection did not fully extract the selected card")
	}
	c.inspect(false)
	settleController(t, c)
	if c.cards[c.selected].lift.value != .4 || c.detail.value != 0 {
		t.Fatal("return did not restore the preview")
	}
	c.navigate(-int(^uint(0)>>1), -int(^uint(0)>>1))
	settleController(t, c)
	if c.selected != (archiveCell{minLane, minRow}) {
		t.Fatal("navigation escaped the archive bounds")
	}
}

func TestEntranceHandsOffAtSelection(t *testing.T) {
	for _, aspect := range []float32{16.0 / 9, 1.6, .8} {
		c := newSceneController()
		start := time.Unix(0, 0)
		c.advance(start, false, true, aspect)
		c.snapshot(nil, aspect)
		c.advance(start.Add(time.Duration(playbackDuration*float64(time.Second))), false, true, aspect)
		if !c.interactive || c.inspecting || c.detail.value != 0 || c.moving() {
			t.Fatal("entrance did not stop at an interactive selection")
		}
		if c.cards[c.selected].lift.value != .4 {
			t.Fatal("entrance opened the selected file")
		}
		want := cameraAt(shotEnd, aspect)
		near := func(a, b vec3) bool {
			d := sub(a, b)
			return dot(d, d) < .000001
		}
		if !near(c.current.eye, want.eye) || !near(c.current.aim, want.aim) || c.current.span != want.span {
			t.Fatal("handoff switched away from the entrance camera")
		}
		c.dirty = false
		if c.advance(start.Add(time.Hour), false, true, aspect) || c.moving() || c.inspecting {
			t.Fatal("entrance restarted or automatically opened a file after settling")
		}
		c.navigate(1, 1)
		settleController(t, c)
		if !near(c.current.forward, want.forward) || c.current.span != want.span {
			t.Fatal("navigation used a different lens or angle")
		}
		c.inspect(true)
		settleController(t, c)
		c.inspect(false)
		settleController(t, c)
		if !near(c.current.forward, want.forward) || c.current.span != want.span {
			t.Fatal("return did not restore the selection camera")
		}
	}
}

func TestInteractiveClockExcludesHiddenTime(t *testing.T) {
	c := newSceneController()
	c.inspect(true)
	start := time.Unix(0, 0)
	c.advance(start, false, true, 16.0/9)
	c.advance(start.Add(time.Second/60), false, true, 16.0/9)
	before := c.card(c.selected)
	c.advance(start.Add(time.Second), true, true, 16.0/9)
	c.advance(start.Add(time.Hour), false, true, 16.0/9)
	if c.card(c.selected) != before {
		t.Fatal("hidden time advanced interactive motion")
	}
	c.advance(start.Add(2*time.Hour), false, false, 16.0/9)
	c.advance(start.Add(3*time.Hour), false, true, 16.0/9)
	if c.card(c.selected) != before {
		t.Fatal("time waiting for a drawable advanced interactive motion")
	}
}

func TestPickingUsesNearestRenderedCard(t *testing.T) {
	c := newSceneController()
	c.hasShown, c.shownAspect = true, 16.0/9
	c.shownCamera = sceneCamera{eye: vec3{0, 1.8, 10}, aim: vec3{0, 1.8, 0}, forward: vec3{0, 0, -1}, span: 8, distance: 10}
	c.shown = []shownCard{
		{cellAt(0), instance{position: [4]float32{0, 0, 0, 0}, rotation: [4]float32{0, 1}}},
		{cellAt(123), instance{position: [4]float32{0, 0, 2, 0}, rotation: [4]float32{float32(math.Sin(.1)), float32(math.Cos(.1))}}},
	}
	if got, hit := c.pick(800, 450, 1600, 900); !hit || got != cellAt(123) {
		t.Fatalf("pick = %+v, %v; want the closest card's coordinate identity", got, hit)
	}
	if _, hit := c.pick(-1, 450, 1600, 900); hit {
		t.Fatal("picked outside the viewport")
	}
	c.shown[1].pose.position[0] = 10000
	if got, hit := c.pick(800, 450, 1600, 900); !hit || got != cellAt(0) {
		t.Fatal("picking did not reveal the card behind")
	}
}

func TestCameraAndLightingPresetsKeepSelectionAndWarmLamp(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	c.activate()
	settleController(t, c)
	selected, near, lamp := c.selected, c.current, c.selectionLamp()
	c.inspect(true)
	settleController(t, c)
	before := c.current
	c.setFarView(true)
	if c.current != before || c.inspecting || c.selected != selected {
		t.Fatal("changing the view jumped the pose or changed the selected file")
	}
	settleController(t, c)
	if c.current.span <= near.span || c.current.forward.y >= near.forward.y {
		t.Fatal("Far did not use a wider, elevated survey camera")
	}
	far := c.current
	c.setDarkMode(true)
	if c.night.value != 0 {
		t.Fatal("dark mode skipped its lighting transition")
	}
	settleController(t, c)
	if c.night.value != 1 || c.current != far || c.selected != selected || c.selectionLamp() != lamp {
		t.Fatal("dark mode changed the camera, selection, or warm underlight")
	}
	c.setFarView(false)
	c.setDarkMode(false)
	settleController(t, c)
	if c.night.value != 0 || c.current.span != near.span || c.current.forward != near.forward || c.active {
		t.Fatal("presets did not return to the settled near/light view")
	}
}

func TestInspectionCanOrbitAndReturnWithoutChangingSelection(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	c.inspect(true)
	settleController(t, c)
	front, selected := c.current, c.selected
	c.turnFile()
	if c.current != front || c.turn.value != 0 || !c.back || !c.turningFile() {
		t.Fatal("turning the file jumped the first frame instead of starting an orbit")
	}
	settleController(t, c)
	if c.turn.value != 1 || c.current.forward.z <= 0 || !c.inspecting || c.selected != selected {
		t.Fatal("orbit did not reach the reverse face of the same extracted file")
	}
	c.turnFile()
	settleController(t, c)
	if c.turn.value != 0 || c.current.forward.z >= 0 || c.back {
		t.Fatal("reverse orbit did not return to the front face")
	}
	c.turnFile()
	settleController(t, c)
	c.inspect(false)
	settleController(t, c)
	if c.inspecting || c.back || c.turn.value != 0 || c.detail.value != 0 || c.selected != selected {
		t.Fatal("Return from the reverse face did not restore the browsing pose")
	}
}
