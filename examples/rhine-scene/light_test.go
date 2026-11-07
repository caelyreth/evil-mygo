package main

import (
	"math"
	"testing"
	"time"
)

func TestSelectionLampFollowsWithoutJumping(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	c.advance(time.Unix(0, 0), false, true, 16.0/9)
	before := c.selectionLamp()
	c.navigate(2, 8)
	if c.selectionLamp() != before {
		t.Fatal("selection teleported the lamp instead of animating it")
	}
	x, z := c.selected.position()
	c.time += 1.0 / 60
	c.step(1.0 / 60)
	during := c.selectionLamp()
	if during[0] <= before[0] || during[0] >= x || during[2] <= before[2] || during[2] >= z {
		t.Fatal("lamp did not move continuously toward the selected slot")
	}
	settleController(t, c)
	want := [4]float32{x, selectionLampHeight(float32(c.cards[c.selected].base.value)), z, selectionLampPower}
	if c.selectionLamp() != want || c.moving() {
		t.Fatal("lamp did not settle under the selected slot")
	}
	preview := c.card(c.selected).position
	if math.Abs(float64(preview[1]-want[1])-.24) > .00001 || want[1] <= selectionLampFloorY+.5 {
		t.Fatal("lamp is buried at floor level instead of just below the preview")
	}
	c.inspect(true)
	settleController(t, c)
	if c.selectionLamp() != want {
		t.Fatal("inspection pulled the underlight away from the preview's lower edge")
	}
}

func TestSelectionLampEntranceAndHiddenTime(t *testing.T) {
	c := newSceneController()
	if light := c.selectionLamp(); light[3] != 0 || light[2] != heroPosition(shotStart).z {
		t.Fatal("lamp did not start dark beneath the entering field")
	}
	c.clock.elapsed = playbackDuration
	before := c.selectionLamp()
	start := time.Unix(0, 0)
	c.advance(start, false, true, 16.0/9)
	if c.selectionLamp() != before {
		t.Fatal("lamp jumped across the entrance handoff")
	}
	c.navigate(1, 2)
	c.advance(start.Add(time.Second/60), false, true, 16.0/9)
	before = c.selectionLamp()
	c.advance(start.Add(time.Second), true, true, 16.0/9)
	c.advance(start.Add(time.Hour), false, true, 16.0/9)
	if c.selectionLamp() != before {
		t.Fatal("time hidden advanced the moving lamp")
	}
	values := sceneGlobals(1280, 800, shotEnd, cameraAt(shotEnd, 1.6))
	if math.Abs(float64(values.underlight[3])-selectionLampPower) > .00001 {
		t.Fatal("scene uniforms omitted the selected-file lamp")
	}
}

func TestStudioLightingDoesNotFollowClickedSlot(t *testing.T) {
	camera := cameraAt(shotEnd, 1.6)
	initial := heroPosition(shotEnd)
	before := sceneGlobalsFor(1280, 800, initial, camera)
	after := sceneGlobalsFor(1280, 800, add(initial, vec3{40, 3, 28}), camera)
	if before.light != after.light || before.lightProjection != after.lightProjection {
		t.Fatal("selecting a different slot changed the studio lighting or shadow coverage")
	}
	camera.eye, camera.aim = add(camera.eye, vec3{100, 0, -80}), add(camera.aim, vec3{100, 0, -80})
	far := sceneGlobalsFor(1280, 800, initial, camera)
	if far.light != before.light || far.lightProjection == before.lightProjection {
		t.Fatal("camera travel changed the lighting direction or failed to carry shadow coverage")
	}
}

func TestSelectionFocusDoesNotJump(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	c.advance(time.Unix(0, 0), false, true, 16.0/9)
	focus := c.focus
	c.navigate(2, 8)
	if c.focus != focus {
		t.Fatal("selection teleported the focus distance")
	}
	settleController(t, c)
	pose := c.card(c.selected).position
	want := dot(sub(vec3{pose[0], pose[1] + 2, pose[2]}, c.current.eye), c.current.forward)
	if math.Abs(c.focus.value-float64(want)) > .00001 || c.moving() {
		t.Fatal("focus did not settle on the selected file")
	}
}
