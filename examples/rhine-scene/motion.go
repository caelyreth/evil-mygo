package main

import (
	"math"
	"time"
)

// The opening wave and camera choreography stop at an interactive file preview.
// Third-party notices for portions of these curves are in THIRD_PARTY_LICENSES.
const (
	shotStart        = 21.9
	shotEnd          = 27.3
	playbackDuration = shotEnd - shotStart
)

func sceneTime(elapsed float64) float64 {
	if elapsed >= playbackDuration {
		return shotEnd
	}
	return shotStart + max(elapsed, 0)
}

func smooth(v float64) float64 {
	v = min(max(v, 0), 1)
	return v * v * v * (10 + v*(-15+6*v))
}

func mix(a, b, t float64) float64   { return a + (b-a)*t }
func bell(x, width float64) float64 { return math.Exp(-.5 * (x / width) * (x / width)) }

func archiveWave(row, lane, shot float64) float64 {
	t := shot - 22
	if t <= 0 || t >= 4.35 {
		return 0
	}
	phase := row + (lane-2)*.65
	packet := func(distance float64) float64 {
		return 2.5*bell(distance, 3.8) - .58*bell(distance-6, 3.5)
	}
	return smooth(t/.32) * (packet(phase-3-t*19)*(1-smooth((t-2.15)/.65)) +
		packet(phase-(32-(t-2.3)*24))*smooth((t-2.17)/.32)*(1-smooth((t-3.5)/.85)))
}

func settlingWave(distance, shot float64) float64 {
	age := shot - 25.05 - math.Abs(distance)*.065
	envelope := math.Max(-.42, 2.15-.17*(math.Sqrt(distance*distance+1)-1))
	ring := 0.0
	if age > 0 {
		ring = math.Sin(age*5.1) * math.Exp(-age*1.3)
	}
	return envelope * (smooth(age/.62) + .18*ring*smooth(age/.16))
}

func field(row, lane, shot float64) float64 {
	handoff := smooth((shot - 24.95) / .45)
	selection := smooth((shot - 25.4) / .95)
	shoulderTime := shot + .3*handoff*(1-selection)
	column := mix(1, .25+.75*bell(lane-2, .55), selection)
	entrance := archiveWave(row, lane, shot)*(1-handoff) + settlingWave(row-12, shoulderTime)*column
	// Land on the interactive field exactly, without a second settling shot.
	return mix(entrance, selectedField(row-12, lane-2), smooth((shot-(shotEnd-.8))/.8))
}

func selectedField(row, lane float64) float64 {
	envelope := math.Max(-.42, 2.15-.17*(math.Sqrt(row*row+1)-1))
	return envelope * (.25 + .75*bell(lane, .55))
}

func extraction(shot float64) float64 {
	return .4 * smooth((shot-25.58)/.82)
}

func entryOffset(shot float64) float32 {
	v := 1 - min(max((shot-21.92)/.75, 0), 1)
	return float32(-23 * v * v)
}

func heroPosition(shot float64) vec3 {
	return vec3{0, float32(-4.6 + field(12, 2, shot) + extraction(shot)), -2.17 + entryOffset(shot)}
}

// Advance only after the first GPU frame, and exclude time spent hidden.
// Each pose is sampled from absolute playback time, independent of frame rate.
type playbackClock struct {
	elapsed  float64
	previous time.Time
}

func (c *playbackClock) advance(now time.Time, hidden, ready bool) bool {
	if hidden || !ready {
		c.previous = time.Time{}
		return false
	}
	before := c.elapsed
	if !c.previous.IsZero() {
		c.elapsed = min(playbackDuration, c.elapsed+max(0, now.Sub(c.previous).Seconds()))
	}
	c.previous = now
	return c.elapsed != before
}

func (c *playbackClock) playing() bool { return c.elapsed < playbackDuration }
