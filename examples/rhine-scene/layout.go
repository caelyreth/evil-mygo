package main

import (
	"math"
	"slices"
)

type instance struct {
	position [4]float32 // Translation; w distinguishes the detailed card.
	rotation [4]float32 // Lean sine/cosine, stable file ID, reserved.
}

const (
	instanceCount       = 11 * 51
	selectionLampFloorY = -4.36
	selectionLampPower  = 144
	shadowMapSize       = 2048
	shadowWorldSpan     = 52
)

func sceneInstances(result []instance, shot float64, forward vec3) []instance {
	if cap(result) < instanceCount {
		result = make([]instance, 0, instanceCount)
	}
	result = result[:0]
	entry := entryOffset(shot)
	for lane := -3; lane <= 7; lane++ {
		for row := -8; row < 43; row++ {
			if lane == 2 && row == 12 {
				continue
			}
			x, z := float32(lane-2)*5.2, (float32(row)-15.5)*.62+entry
			y := float32(-4.6 + field(float64(row), float64(lane), shot))
			angle := (field(float64(row)+.5, float64(lane), shot) - field(float64(row)-.5, float64(lane), shot)) * .024
			result = append(result, instance{position: [4]float32{x, y, z, 0}, rotation: [4]float32{float32(math.Sin(angle)), float32(math.Cos(angle)), float32((archiveCell{lane, row}).number())}})
		}
	}
	// Keep the detailed card at a stable buffer index. The glass pass inserts
	// it between the far and near array batches for the current camera.
	slices.SortFunc(result, func(a, b instance) int {
		da, db := instanceDepth(a, forward), instanceDepth(b, forward)
		if da > db {
			return -1
		}
		if da < db {
			return 1
		}
		return 0
	})
	hero := heroPosition(shot)
	angle := (field(12.5, 2, shot) - field(11.5, 2, shot)) * .024 * (1 - smooth(extraction(shot)/.4))
	return append(result, instance{position: [4]float32{hero.x, hero.y, hero.z, 1}, rotation: [4]float32{float32(math.Sin(angle)), float32(math.Cos(angle)), float32((archiveCell{2, 12}).number())}})
}

func instanceDepth(i instance, forward vec3) float32 {
	return dot(vec3{i.position[0], i.position[1], i.position[2]}, forward)
}

func glassSplit(instances []instance, forward vec3) int {
	heroDepth := instanceDepth(instances[len(instances)-1], forward)
	for i, placement := range instances[:len(instances)-1] {
		if instanceDepth(placement, forward) < heroDepth {
			return i
		}
	}
	return len(instances) - 1
}

type sceneCamera struct {
	eye, aim, forward vec3
	span, distance    float32
}

func cameraAt(shot float64, aspect float32) sceneCamera {
	shot = min(max(shot, shotStart), shotEnd)
	orbit := smooth((shot - 22.6) / 1.6)
	settle := smooth((shot - 24.25) / 2.25)
	yaw := (89 - 22*orbit - 8*settle) * math.Pi / 180
	elevation := (3 + 40*smooth((shot-21.96)/.22) - 8*orbit - 16*settle) * math.Pi / 180
	direction := vec3{float32(-math.Sin(yaw) * math.Cos(elevation)), float32(math.Sin(elevation)), float32(math.Cos(yaw) * math.Cos(elevation))}
	span := mix(mix(10.8, 10.3, orbit), 7.33, settle)
	distance := mix(28+7*orbit, 140, settle)
	openingAspect := float64(aspect) / (16.0 / 9)
	openingSpan := func(span float64) float64 { return span / min(1, openingAspect) }
	aim := vec3{-1.091, float32(mix(-2.55+.4*orbit, -.045, settle)), float32(mix(2.48, .481, settle))}
	right := unit(cross(vec3{y: 1}, direction))
	up := cross(direction, right)
	aim = add(aim, mul(right, float32(-2.05*(1-smooth((shot-25.4)/.95))*smooth((shot-24.2)/.8))))
	anchor := func(screenX, screenY, span float64) vec3 {
		pixelScale := 1080 / openingSpan(span)
		p := add(heroPosition(shot), vec3{-2.5, 3.7, 0})
		p = add(p, mul(right, float32(-(screenX-960)*openingAspect/pixelScale)))
		return add(p, mul(up, float32(-(540-screenY)/pixelScale)))
	}
	if shot >= 25.05 {
		pan := smooth((shot - 25.4) / 1.05)
		// Lower selection framing without changing the viewing angle or zoom.
		target := anchor(mix(840, 518, pan), mix(340, 360, pan), span)
		aim = add(aim, mul(sub(target, aim), float32(smooth((shot-25.05)/.35))))
	}
	eye := add(aim, mul(direction, float32(distance)))
	return sceneCamera{eye: eye, aim: aim, forward: mul(direction, -1), span: float32(openingSpan(span)), distance: float32(distance)}
}

func selectionCamera(cell archiveCell, base, entry float64, aspect float32) sceneCamera {
	// The near preset preserves the entrance's final framing.
	camera := cameraAt(shotEnd, aspect)
	x, z := cell.position()
	initial := heroPosition(shotEnd)
	offset := vec3{x - initial.x, float32(base+.4) - initial.y, z + float32(entry) - initial.z}
	camera.eye, camera.aim = add(camera.eye, offset), add(camera.aim, offset)
	return camera
}

func surveyCamera(cell archiveCell, base, entry float64, aspect float32) sceneCamera {
	x, z := cell.position()
	direction := unit(vec3{-.82, .95, 1.1})
	aim := vec3{x, float32(base) + 1.5, z + float32(entry)}
	const distance = float32(165)
	span := float32(15) * max(1, (16.0/9)/aspect)
	return sceneCamera{eye: add(aim, mul(direction, distance)), aim: aim,
		forward: mul(direction, -1), span: span, distance: distance}
}

type globals struct {
	viewProjection  matrix
	lightProjection matrix
	camera          [4]float32
	right           [4]float32
	up              [4]float32
	forward         [4]float32
	lens            [4]float32
	size            [4]float32 // Pixel size, focus depth, fog distance.
	light           [4]float32 // Fixed studio direction; w is the night-mode blend.
	underlight      [4]float32 // Moving lamp XYZ and radiant power.
	pages           [4]float32 // Native-page coverage, reserved.
}

func sceneGlobals(width, height int, shot float64, camera sceneCamera) globals {
	hero := heroPosition(shot)
	values := sceneGlobalsFor(width, height, hero, camera)
	values.underlight = entranceLamp(shot, hero)
	return values
}

func entranceLamp(shot float64, hero vec3) [4]float32 {
	base := hero.y - float32(extraction(shot))
	return [4]float32{hero.x, selectionLampHeight(base), hero.z, selectionLampPower * float32(smooth((shot-25.05)/1.2))}
}

func selectionLampHeight(base float32) float32 {
	// Just below the preview's lower edge, not hidden deep under the archive.
	return max(selectionLampFloorY, base+.16)
}

func sceneGlobalsFor(width, height int, hero vec3, camera sceneCamera) globals {
	aspect := float32(width) / float32(height)
	right, up, forward, view := lookAt(camera.eye, camera.aim)
	tan := camera.span / (2 * camera.distance)
	const near, far = float32(5), float32(300)
	projection := matrix{0: 1 / (tan * aspect), 5: 1 / tan, 10: far / (far - near), 11: 1, 14: -far * near / (far - near)}
	// Illumination is directional and does not recenter on a clicked slot.
	// Only shadow coverage follows the rendered camera, on a stable texel grid.
	lightDirection := unit(vec3{-8, 15.5, 7})
	lightAim := vec3{camera.aim.x, -1.5, camera.aim.z}
	lightRight, lightUp, _, _ := lookAt(lightDirection, vec3{})
	const texel = float64(shadowWorldSpan) / shadowMapSize
	for _, axis := range [2]vec3{lightRight, lightUp} {
		coordinate := float64(dot(lightAim, axis))
		lightAim = add(lightAim, mul(axis, float32(math.Round(coordinate/texel)*texel-coordinate)))
	}
	lightEye := add(lightAim, mul(lightDirection, 28))
	_, _, _, lightView := lookAt(lightEye, lightAim)
	lightProjection := matrix{0: 2.0 / shadowWorldSpan, 5: 2.0 / shadowWorldSpan, 10: 1.0 / 65, 15: 1}
	focus := dot(sub(add(hero, vec3{y: 2}), camera.eye), forward)
	return globals{
		viewProjection: product(projection, view), lightProjection: product(lightProjection, lightView),
		camera: [4]float32{camera.eye.x, camera.eye.y, camera.eye.z, 1}, right: [4]float32{right.x, right.y, right.z, 0},
		up: [4]float32{up.x, up.y, up.z, 0}, forward: [4]float32{forward.x, forward.y, forward.z, 0},
		lens: [4]float32{tan, aspect, near, far}, size: [4]float32{float32(width), float32(height), focus, camera.distance},
		light:      [4]float32{lightDirection.x, lightDirection.y, lightDirection.z, 0},
		underlight: entranceLamp(shotEnd, hero),
	}
}

func lookAt(eye, aim vec3) (right, up, forward vec3, view matrix) {
	forward = unit(sub(aim, eye))
	right = unit(cross(forward, vec3{y: 1}))
	up = cross(right, forward)
	view = matrix{right.x, up.x, forward.x, 0, right.y, up.y, forward.y, 0, right.z, up.z, forward.z, 0, -dot(right, eye), -dot(up, eye), -dot(forward, eye), 1}
	return
}
