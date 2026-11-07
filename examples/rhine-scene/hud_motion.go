//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"time"

	"github.com/egoist/mygo/ui"
)

var (
	archiveHUDMotion = ui.Animation{Spring: ui.SpringOptions{Duration: 300 * time.Millisecond}}
	archiveHUDColor  = ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.EaseOut, NonMotion: true, Oklab: true}
	archiveHUDLayout = ui.ElementTransition{Position: true, Size: true, Animation: &archiveHUDMotion}
	archiveHUDText   = ui.ContentTransition{Animation: &archiveHUDMotion}
)

func archiveHUDNumber(down bool) ui.ContentTransition {
	return ui.ContentTransition{Numeric: true, Down: down, Animation: &archiveHUDMotion}
}

func animateHUDButton(button ui.Element, face, ink ui.Color) ui.Color {
	button.Background(button.AnimateColor("face", face, archiveHUDColor))
	scale := float32(1)
	if button.Pressed() {
		scale = .96
	}
	button.AnimatePose(ui.Pose{Scale: scale}, archiveHUDMotion)
	return button.AnimateColor("ink", ink, archiveHUDColor)
}
