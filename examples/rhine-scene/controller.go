package main

import (
	"math"
	"slices"
	"time"
)

type spring struct{ value, velocity float64 }

func (s *spring) step(target, rate, dt float64) bool {
	delta := s.value - target
	impulse := s.velocity + rate*delta
	decay := math.Exp(-rate * dt)
	s.value = target + (delta+impulse*dt)*decay
	s.velocity = (s.velocity - rate*impulse*dt) * decay
	if math.Abs(s.value-target) < .0002 && math.Abs(s.velocity) < .001 {
		s.value, s.velocity = target, 0
		return false
	}
	return true
}

type cardMotion struct{ base, lift, angle spring }
type selectionPulse struct {
	cell archiveCell
	at   float64
}

type shownCard struct {
	cell archiveCell
	pose instance
}

// All commands share this state; the HUD never maintains its own selection.
// Cards keep coordinate identities even when their GPU instances are sorted.
type sceneController struct {
	selected    archiveCell
	interactive bool
	inspecting  bool
	back        bool
	turn        spring
	farView     bool
	darkMode    bool
	night       spring
	dirty       bool
	clock       playbackClock
	previous    time.Time
	time        float64
	active      bool
	entry       spring
	detail      spring
	cards       map[archiveCell]*cardMotion
	eye, aim    [3]spring
	span        spring
	focus       spring
	lamp        [4]spring // XYZ and power; follows the preview's lower edge.
	pulses      []selectionPulse
	aspect      float32
	current     sceneCamera
	// These are the last encoded poses, not predictions for the next frame.
	shown       []shownCard
	shownCamera sceneCamera
	shownAspect float32
	shownShot   float64
	hasShown    bool
}

func newSceneController() *sceneController {
	return &sceneController{selected: archiveCell{2, 12}, dirty: true, aspect: 16.0 / 9,
		cards: make(map[archiveCell]*cardMotion, residentCapacity+1), shown: make([]shownCard, 0, residentCapacity)}
}

func (c *sceneController) inspectingNow() bool {
	return c.inspecting
}

func (c *sceneController) moving() bool {
	return (!c.interactive && c.clock.playing()) || (c.interactive && c.active)
}

func (c *sceneController) advance(now time.Time, hidden, ready bool, aspect float32) bool {
	resized := c.aspect != aspect
	c.aspect = aspect
	if !c.interactive {
		changed := c.clock.advance(now, hidden, ready)
		if !hidden && ready && !c.clock.playing() {
			c.activateFrom(false)
			c.step(0)
			c.previous = now
		}
		c.dirty = c.dirty || changed || resized
		return c.dirty
	}
	if hidden || !ready {
		c.previous = time.Time{}
		return c.dirty || resized
	}
	dt := 0.0
	if !c.previous.IsZero() {
		dt = max(0, now.Sub(c.previous).Seconds())
	}
	c.previous = now
	if dt > 0 || resized {
		// Bound a stalled visible frame without changing the spring's stability.
		dt = min(dt, .1)
		c.time += dt
		wasActive := c.active
		c.step(dt)
		c.dirty = c.dirty || wasActive || c.active || resized
	}
	return c.dirty
}

func (c *sceneController) activate() {
	c.activateFrom(true)
}

func (c *sceneController) activateFrom(useShown bool) {
	if c.interactive {
		return
	}
	shot := sceneTime(c.clock.elapsed)
	camera := cameraAt(shot, c.aspect)
	if useShown && c.hasShown {
		shot, camera = c.shownShot, c.shownCamera
	}
	c.inspecting, c.back, c.turn = false, false, spring{}
	seed := func(cell archiveCell, pose instance) {
		lift := 0.0
		if cell == c.selected {
			lift = extraction(shot)
		}
		c.cards[cell] = &cardMotion{base: spring{value: float64(pose.position[1]) - lift}, lift: spring{value: lift},
			angle: spring{value: math.Atan2(float64(pose.rotation[0]), float64(pose.rotation[1]))}}
	}
	seed(c.selected, c.card(c.selected))
	if c.hasShown {
		for _, shown := range c.shown {
			pose := shown.pose
			if !useShown {
				pose = c.card(shown.cell)
			}
			seed(shown.cell, pose)
		}
	}
	c.entry = spring{value: float64(entryOffset(shot))}
	c.detail = spring{}
	c.eye = [3]spring{{value: float64(camera.eye.x)}, {value: float64(camera.eye.y)}, {value: float64(camera.eye.z)}}
	c.aim = [3]spring{{value: float64(camera.aim.x)}, {value: float64(camera.aim.y)}, {value: float64(camera.aim.z)}}
	c.span = spring{value: float64(camera.span)}
	selected := c.card(c.selected).position
	if useShown {
		for _, shown := range c.shown {
			if shown.cell == c.selected {
				selected = shown.pose.position
				break
			}
		}
	}
	c.focus = spring{value: float64(dot(sub(vec3{selected[0], selected[1] + 2, selected[2]}, camera.eye), camera.forward))}
	x, z := c.selected.position()
	lamp := entranceLamp(shot, vec3{x, selected[1], z + entryOffset(shot)})
	for axis, value := range lamp {
		c.lamp[axis] = spring{value: float64(value)}
	}
	c.current = camera
	c.interactive, c.active, c.dirty = true, true, true
	c.clock.elapsed, c.clock.previous = playbackDuration, time.Time{}
	c.previous = time.Time{}
}

func (c *sceneController) selectCell(cell archiveCell) {
	if !cell.valid() {
		return
	}
	c.activate()
	if cell != c.selected {
		c.selected, c.inspecting, c.back = cell, false, false
		c.ensureCard(cell)
		c.pulses = append(c.pulses, selectionPulse{cell, c.time})
		if len(c.pulses) > 6 {
			c.pulses = c.pulses[len(c.pulses)-6:]
		}
	}
	c.wake()
}

func (c *sceneController) wake() {
	if !c.active {
		c.previous = time.Time{}
	}
	c.active, c.dirty = true, true
}

func (c *sceneController) navigate(lane, row int) {
	// Clamp the delta before addition, so large scroll/key bursts cannot wrap.
	target := archiveCell{c.selected.lane + min(max(lane, minLane-c.selected.lane), maxLane-c.selected.lane),
		c.selected.row + min(max(row, minRow-c.selected.row), maxRow-c.selected.row)}
	if target != c.selected {
		c.selectCell(target)
	}
}

func (c *sceneController) inspect(on bool) {
	c.activate()
	if !on || !c.inspecting {
		c.back = false
	}
	c.inspecting = on
	c.wake()
}

func (c *sceneController) turnFile() {
	if !c.inspecting || c.detail.value < .85 {
		return
	}
	c.back = !c.back
	c.wake()
}

func (c *sceneController) turningFile() bool {
	target := 0.0
	if c.inspecting && c.back {
		target = 1
	}
	return c.turn.value != target || c.turn.velocity != 0
}

func (c *sceneController) setFarView(far bool) {
	if c.farView == far {
		return
	}
	c.activate()
	c.farView, c.inspecting, c.back = far, false, false
	c.wake()
}

func (c *sceneController) setDarkMode(dark bool) {
	if c.darkMode == dark {
		return
	}
	c.activate()
	c.darkMode = dark
	c.wake()
}

func (c *sceneController) height(row, lane float64) float64 {
	distance := row - float64(c.selected.row)
	height := selectedField(distance, lane-float64(c.selected.lane))
	for _, pulse := range c.pulses {
		age := c.time - pulse.at
		if age < 0 || age > 3.2 {
			continue
		}
		d := math.Hypot(row-float64(pulse.cell.row), (lane-float64(pulse.cell.lane))*2.2)
		height += .8 * smooth(age/.2) * math.Exp(-age*1.15) * math.Cos((d-age*8)*.58) * bell(d-age*8, 3.4)
	}
	return -4.6 + height
}

func (c *sceneController) step(dt float64) {
	c.active = c.entry.step(0, 5, dt)
	night := 0.0
	if c.darkMode {
		night = 1
	}
	c.active = c.night.step(night, 6, dt) || c.active
	turn := 0.0
	if c.inspecting && c.back {
		turn = 1
	}
	c.active = c.turn.step(turn, 5, dt) || c.active
	c.ensureCard(c.selected)
	for cell, card := range c.cards {
		lift := 0.0
		if cell == c.selected {
			lift = .4
			if c.inspecting {
				lift = 4.05
			}
		}
		base := c.height(float64(cell.row), float64(cell.lane))
		angle := (c.height(float64(cell.row)+.5, float64(cell.lane)) - c.height(float64(cell.row)-.5, float64(cell.lane))) * .024 * (1 - c.detail.value)
		if cell == c.selected {
			angle *= 1 - smooth(card.lift.value/.4)
		}
		a, b, d := card.base.step(base, 7, dt), card.lift.step(lift, 5, dt), card.angle.step(angle, 7, dt)
		c.active = c.active || a || b || d
	}
	lift := c.cards[c.selected].lift.value
	detail := smooth((lift - .4) / 3.65)
	c.active = c.detail.step(detail, 5, dt) || c.active
	pose := c.card(c.selected)
	for axis, target := range [4]float32{pose.position[0], selectionLampHeight(float32(c.cards[c.selected].base.value)), pose.position[2], selectionLampPower} {
		c.active = c.lamp[axis].step(float64(target), 7, dt) || c.active
	}
	browse := selectionCamera(c.selected, c.cards[c.selected].base.value, c.entry.value, c.aspect)
	if c.farView {
		browse = surveyCamera(c.selected, c.cards[c.selected].base.value, c.entry.value, c.aspect)
	}
	closeDirection := unit(vec3{-.277, .238, .931})
	// Animate an azimuth, not an interpolation between opposite eye positions:
	// the inspection camera orbits around the file instead of crossing through it.
	yaw := math.Atan2(float64(closeDirection.x), float64(closeDirection.z)) + math.Pi*c.turn.value
	horizontal := math.Sqrt(1 - float64(closeDirection.y*closeDirection.y))
	closeDirection.x, closeDirection.z = float32(math.Sin(yaw)*horizontal), float32(math.Cos(yaw)*horizontal)
	direction := unit(add(mul(browse.forward, -float32(1-c.detail.value)), mul(closeDirection, float32(c.detail.value))))
	distance := float32(mix(float64(browse.distance), 72, c.detail.value))
	closeSpan := float32(6.2) * max(1, (16.0/9)/c.aspect)
	span := float32(mix(float64(browse.span), float64(closeSpan), c.detail.value))
	up := unit(cross(closeDirection, unit(cross(vec3{y: 1}, closeDirection))))
	center := vec3{pose.position[0], float32(c.cards[c.selected].base.value+mix(.4, lift, c.detail.value)) + 1.85, pose.position[2]}
	closeAim := add(center, mul(up, -.055*closeSpan))
	aim := add(browse.aim, mul(sub(closeAim, browse.aim), float32(c.detail.value)))
	minX, maxX, minZ, maxZ := cameraLimits()
	for axis, bounds := range [2][2]float64{{minX, maxX}, {minZ, maxZ}} {
		lamp := &c.lamp[axis*2]
		bound := min(max(lamp.value, bounds[0]), bounds[1])
		if bound != lamp.value {
			lamp.value, lamp.velocity = bound, 0
		}
	}
	aim.x, aim.z = float32(min(max(float64(aim.x), minX), maxX)), float32(min(max(float64(aim.z), minZ), maxZ))
	eye := add(aim, mul(direction, distance))
	for axis, target := range [3]float32{eye.x, eye.y, eye.z} {
		c.active = c.eye[axis].step(float64(target), 5, dt) || c.active
	}
	for axis, target := range [3]float32{aim.x, aim.y, aim.z} {
		c.active = c.aim[axis].step(float64(target), 5, dt) || c.active
	}
	c.active = c.span.step(float64(span), 5, dt) || c.active
	c.capCamera(0, minX, maxX)
	c.capCamera(2, minZ, maxZ)
	c.current.eye = vec3{float32(c.eye[0].value), float32(c.eye[1].value), float32(c.eye[2].value)}
	c.current.aim = vec3{float32(c.aim[0].value), float32(c.aim[1].value), float32(c.aim[2].value)}
	delta := sub(c.current.aim, c.current.eye)
	c.current.forward, c.current.distance, c.current.span = unit(delta), float32(math.Sqrt(float64(dot(delta, delta)))), float32(c.span.value)
	focus := dot(sub(vec3{pose.position[0], pose.position[1] + 2, pose.position[2]}, c.current.eye), c.current.forward)
	c.active = c.focus.step(float64(focus), 7, dt) || c.active
	c.pulses = slices.DeleteFunc(c.pulses, func(p selectionPulse) bool { return c.time-p.at > 3.2 })
	c.active = c.active || len(c.pulses) > 0
}

func (c *sceneController) selectionLamp() [4]float32 {
	if !c.interactive {
		pose := c.card(c.selected).position
		return entranceLamp(sceneTime(c.clock.elapsed), vec3{pose[0], pose[1], pose[2]})
	}
	return [4]float32{float32(c.lamp[0].value), float32(c.lamp[1].value), float32(c.lamp[2].value), float32(c.lamp[3].value)}
}

func (c *sceneController) ensureCard(cell archiveCell) *cardMotion {
	if card := c.cards[cell]; card != nil {
		return card
	}
	card := &cardMotion{base: spring{value: c.height(float64(cell.row), float64(cell.lane))}}
	c.cards[cell] = card
	return card
}

func (c *sceneController) capCamera(axis int, lo, hi float64) {
	s := &c.aim[axis]
	bound := min(max(s.value, lo), hi)
	shift := bound - s.value
	if shift == 0 {
		return
	}
	s.value = bound
	c.eye[axis].value += shift
	if s.velocity*shift < 0 {
		s.velocity = 0
	}
	if c.eye[axis].velocity*shift < 0 {
		c.eye[axis].velocity = 0
	}
}

func (c *sceneController) card(cell archiveCell) instance {
	x, z := cell.position()
	if !c.interactive {
		shot := sceneTime(c.clock.elapsed)
		lift, detailed := 0.0, float32(0)
		if cell == c.selected {
			lift, detailed = extraction(shot), 1
		}
		angle := (field(float64(cell.row)+.5, float64(cell.lane), shot) - field(float64(cell.row)-.5, float64(cell.lane), shot)) * .024
		if detailed > 0 {
			angle *= 1 - smooth(lift/.4)
		}
		return instance{position: [4]float32{x, float32(-4.6 + field(float64(cell.row), float64(cell.lane), shot) + lift), z + entryOffset(shot), detailed}, rotation: [4]float32{float32(math.Sin(angle)), float32(math.Cos(angle)), float32(cell.number())}}
	}
	card := c.ensureCard(cell)
	detailed := float32(0)
	if cell == c.selected || card.lift.value > .05 {
		detailed = 1
	}
	return instance{position: [4]float32{x, float32(card.base.value + card.lift.value), z + float32(c.entry.value), detailed}, rotation: [4]float32{float32(math.Sin(card.angle.value)), float32(math.Cos(card.angle.value)), float32(cell.number())}}
}

func (c *sceneController) snapshot(result []instance, aspect float32) ([]instance, sceneCamera, vec3) {
	if cap(result) < residentCapacity {
		result = make([]instance, 0, residentCapacity)
	}
	result = result[:0]
	camera := c.current
	if !c.interactive {
		camera = cameraAt(sceneTime(c.clock.elapsed), aspect)
	}
	hero := c.card(c.selected).position
	entry := float32(c.entry.value)
	if !c.interactive {
		entry = entryOffset(sceneTime(c.clock.elapsed))
	}
	window := windowAt(camera, entry)
	right, up, forward, _ := lookAt(camera.eye, camera.aim)
	tan := camera.span / (2 * camera.distance)
	const radius = float32(3.3)
	padX := radius * float32(math.Sqrt(float64(1+tan*tan*aspect*aspect)))
	padY := radius * float32(math.Sqrt(float64(1+tan*tan)))
	c.shown = c.shown[:0]
	for lane := window.minLane; lane <= window.maxLane; lane++ {
		for row := window.minRow; row <= window.maxRow; row++ {
			cell := archiveCell{lane, row}
			pose := c.card(cell)
			center := vec3{pose.position[0], pose.position[1] + 1.85, pose.position[2]}
			delta := sub(center, camera.eye)
			depth := dot(delta, forward)
			// Conservative card bounds include its tilt and optical edges. Keep
			// offscreen shadow casters near the focus as well as visible cards.
			visible := depth+radius >= 5 && depth-radius <= camera.distance+30 &&
				float32(math.Abs(float64(dot(delta, right)))) <= max(depth, 0)*tan*aspect+padX &&
				float32(math.Abs(float64(dot(delta, up)))) <= max(depth, 0)*tan+padY
			shadow := math.Abs(float64(center.x-camera.aim.x)) < 32 && math.Abs(float64(center.z-camera.aim.z)) < 32
			if !visible && !shadow {
				continue
			}
			result = append(result, pose)
			c.shown = append(c.shown, shownCard{cell, pose})
		}
	}
	for cell, card := range c.cards {
		if !window.contains(cell) && cell != c.selected && card.lift.value < .05 && math.Abs(card.lift.velocity) < .001 {
			delete(c.cards, cell)
		}
	}
	c.shownCamera, c.shownAspect, c.hasShown = camera, aspect, true
	c.shownShot = sceneTime(c.clock.elapsed)
	slices.SortFunc(result, func(a, b instance) int {
		da, db := instanceDepth(a, camera.forward), instanceDepth(b, camera.forward)
		if da > db {
			return -1
		}
		if da < db {
			return 1
		}
		return 0
	})
	return result, camera, vec3{hero[0], hero[1], hero[2]}
}

// Picking uses the last rendered camera and rigid card boxes. The ray and
// geometry stay on the CPU; no GPU readback or instance-index identity is used.
func (c *sceneController) pick(x, y, width, height float32) (archiveCell, bool) {
	if !c.hasShown || width <= 0 || height <= 0 || x < 0 || y < 0 || x >= width || y >= height {
		return archiveCell{}, false
	}
	cam := c.shownCamera
	right, up, forward, _ := lookAt(cam.eye, cam.aim)
	tan := cam.span / (2 * cam.distance)
	ray := unit(add(forward, add(mul(right, (2*x/width-1)*tan*c.shownAspect), mul(up, (1-2*y/height)*tan))))
	nearest := float32(math.Inf(1))
	if ray.y < 0 {
		if floor := (-4.63 - cam.eye.y) / ray.y; floor > 0 {
			nearest = floor
		}
	}
	selected := archiveCell{}
	found := false
	for _, shown := range c.shown {
		pose := shown.pose
		sin, cos := pose.rotation[0], pose.rotation[1]
		inverse := func(v vec3) vec3 { return vec3{v.x, v.y*cos + v.z*sin, -v.y*sin + v.z*cos} }
		origin := inverse(sub(cam.eye, vec3{pose.position[0], pose.position[1], pose.position[2]}))
		direction := inverse(ray)
		enter, exit := float32(0), float32(math.Inf(1))
		for axis, bounds := range [3][2]float32{{-2.5, 2.5}, {0, 3.76}, {-.27, .27}} {
			o, d := [3]float32{origin.x, origin.y, origin.z}[axis], [3]float32{direction.x, direction.y, direction.z}[axis]
			if float32(math.Abs(float64(d))) < 1e-7 {
				if o < bounds[0] || o > bounds[1] {
					exit = -1
					break
				}
				continue
			}
			a, b := (bounds[0]-o)/d, (bounds[1]-o)/d
			enter, exit = max(enter, min(a, b)), min(exit, max(a, b))
		}
		depth := enter * dot(ray, forward)
		if exit >= enter && depth >= 5 && depth <= 300 && enter < nearest {
			nearest, selected, found = enter, shown.cell, true
		}
	}
	// Guard cards occlude normally, but cannot pull selection or the camera
	// outside the catalogue. Never pick through them to a farther card.
	return selected, found && selected.valid()
}
