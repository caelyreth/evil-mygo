package main

import (
	"math"
	"testing"
	"time"
)

func TestMotionTracks(t *testing.T) {
	for _, shot := range []float64{21.9, 22, 26.35, 35} {
		if archiveWave(12, 2, shot) != 0 {
			t.Fatalf("scan wave survives outside its authored interval at %v", shot)
		}
	}
	if archiveWave(12.5, 2, 22.5) < 2 || archiveWave(12.5, 2, 23) > 1 {
		t.Fatal("scan crest does not travel along the rows")
	}
	for _, sample := range []struct{ shot, lift float64 }{{25.58, 0}, {25.99, .2}, {26.4, .4}, {shotEnd, .4}, {35, .4}} {
		if got := extraction(sample.shot); math.Abs(got-sample.lift) > 1e-8 {
			t.Fatalf("extraction(%v) = %v, want %v", sample.shot, got, sample.lift)
		}
	}
	if entryOffset(shotStart) != -23 || entryOffset(22.67) != 0 || extraction(shotEnd+30) != .4 {
		t.Fatal("entrance did not stop at selection")
	}
	if math.Abs(field(12, 2, 35)-2.15) > .00001 || math.Abs(field(12, 0, 35)-2.15*.25) > .003 {
		t.Fatal("settling field does not concentrate on the selected column")
	}
	if sceneTime(-1) != shotStart || sceneTime(playbackDuration+10) != shotEnd {
		t.Fatal("playback does not clamp to its authored segment")
	}
}

func TestAnimatedPlacementAndCamera(t *testing.T) {
	var instances []instance
	for _, aspect := range []float32{16.0 / 9, 1.6, .625} {
		for shot := shotStart; shot <= shotEnd; shot += .1 {
			camera := cameraAt(shot, aspect)
			instances = sceneInstances(instances, shot, camera.forward)
			if len(instances) != instanceCount {
				t.Fatalf("instance count changes at %v: %d", shot, len(instances))
			}
			hero := instances[len(instances)-1]
			want := heroPosition(shot)
			if hero.position != [4]float32{want.x, want.y, want.z, 1} {
				t.Fatal("camera and geometry use different hero tracks")
			}
			split := glassSplit(instances, camera.forward)
			previous := float32(math.Inf(1))
			for i, placement := range instances[:len(instances)-1] {
				depth := instanceDepth(placement, camera.forward)
				if depth > previous {
					t.Fatalf("glass not back-to-front at %v", shot)
				}
				previous = depth
				if (i < split) != (depth >= instanceDepth(hero, camera.forward)) {
					t.Fatal("hero glass is inserted at the wrong depth")
				}
				if placement.position[0] == hero.position[0] && placement.position[2] == hero.position[2] {
					t.Fatal("the selected slot contains two cards")
				}
			}
			g := sceneGlobals(int(800*aspect), 800, shot, camera)
			for _, value := range g.viewProjection {
				if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
					t.Fatalf("invalid camera at %v", shot)
				}
			}
			if g.size[2] <= g.lens[2] || g.size[2] >= g.lens[3] {
				t.Fatalf("focus lies outside camera clipping planes at %v", shot)
			}
		}
	}
	for _, shot := range []float64{22.6, 24.25, 25.05, 25.4, shotEnd} {
		before, after := cameraAt(shot-.00001, 16.0/9), cameraAt(shot+.00001, 16.0/9)
		if delta := sub(before.eye, after.eye); dot(delta, delta) > .0001 {
			t.Fatalf("camera jumps across track boundary %v: %+v", shot, delta)
		}
	}
}

func TestPlaybackClock(t *testing.T) {
	start := time.Unix(0, 0)
	for _, fps := range []int{30, 60, 120} {
		var clock playbackClock
		clock.advance(start, false, false)
		clock.advance(start.Add(time.Hour), false, true)
		for frame := 1; frame <= fps*3; frame++ {
			clock.advance(start.Add(time.Hour+time.Duration(frame)*time.Second/time.Duration(fps)), false, true)
		}
		if math.Abs(clock.elapsed-3) > .000001 {
			t.Fatalf("%d FPS: playback time = %v, want 3", fps, clock.elapsed)
		}
		clock.advance(start.Add(2*time.Hour), true, true)
		clock.advance(start.Add(3*time.Hour), false, true)
		if math.Abs(clock.elapsed-3) > .000001 {
			t.Fatal("time hidden advanced the animation")
		}
		clock.advance(start.Add(4*time.Hour), false, true)
		if clock.elapsed != playbackDuration || clock.playing() {
			t.Fatal("clock does not stop at the final pose")
		}
	}
}
