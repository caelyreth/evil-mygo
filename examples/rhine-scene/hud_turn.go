//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"math"

	"github.com/egoist/mygo/ui"
)

const flipControlWidth = float32(96)

type turnControlAnchor struct {
	cell                archiveCell
	width, height, x, y float32
	valid               bool
}

func (a *archiveApp) turnControl(c *ui.Context, width, height float32) {
	show := float32(0)
	if !a.controller.inspectingNow() {
		a.turnAnchor.valid = false
	}
	// Finish the report reveal and near-complete camera close-in before the
	// secondary action fades in; keep it present during subsequent flips.
	if a.controller.inspectingNow() && a.controller.detail.value > .99 && a.presentation.interactive() {
		show = 1
		if anchor := a.turnAnchor; !anchor.valid || anchor.cell != a.controller.selected || anchor.width != width || anchor.height != height {
			a.positionTurnControl(width, height)
		}
	}
	b := ui.ButtonBase(c.Key("turn-file")).Label("Flip").Description("Rotate the camera to the other side of the file").Role(ui.RoleButton).
		Absolute().Left(a.turnAnchor.x).Top(a.turnAnchor.y).Size(flipControlWidth, 36).Padding(0, 12).Gap(8).
		AlignItems(ui.Center).Radius(0).Cursor(ui.CursorPointer).Transition(archiveHUDLayout)
	presence := b.AnimateTo("presence", show, archiveHUDMotion)
	b.Opacity(presence).Translate(0, 10*(1-presence)).Disabled(show == 0 || presence < .9).
		DimDisabled(false).IgnoreInput(show == 0).AccessHidden(show == 0)
	face, ink := panelInk, panelPaper
	if b.Hovered() {
		face = panelGreen
	}
	if b.Pressed() {
		face = panelRust
	}
	ink = animateHUDButton(b, face, ink)
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: 3, H: r.H}, panelAccent, 0)
	}).Children(func() {
		ui.Box(c).Size(18, 18).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			x, y := r.X+r.W/2, r.Y+r.H/2
			for i := range 16 {
				a, b := float64(i)*math.Pi/12, float64(i+1)*math.Pi/12
				p.Line(x+7*float32(math.Cos(a)), y+7*float32(math.Sin(a)),
					x+7*float32(math.Cos(b)), y+7*float32(math.Sin(b)), 1.5, ink)
			}
			p.Line(x-3.5, y-6, x-7, y-3, 1.5, ink)
			p.Line(x-3.5, y-6, x+.5, y-3.5, 1.5, ink)
		})
		ui.Text(c.Key("turn-file-label"), "Flip").Font(archiveDisplayFont).FontSize(12).FontWeight(800).
			LetterSpacing(.6).TextColor(ink).SingleLine()
	}).OnClick(func() { a.command(a.controller.turnFile) })
}

func (a *archiveApp) positionTurnControl(width, height float32) {
	controller := a.controller
	if !controller.hasShown {
		return
	}
	for _, shown := range controller.shown {
		if shown.cell != controller.selected {
			continue
		}
		camera := controller.shownCamera
		right, up, forward, _ := lookAt(camera.eye, camera.aim)
		tan := camera.span / (2 * camera.distance)
		minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
		maxX, maxY := -minX, -minY
		for _, x := range []float32{-2.5, 2.5} {
			for _, y := range []float32{0, 3.7} {
				sin, cos := shown.pose.rotation[0], shown.pose.rotation[1]
				world := vec3{x + shown.pose.position[0], y*cos + shown.pose.position[1], y*sin + shown.pose.position[2]}
				delta := sub(world, camera.eye)
				depth := dot(delta, forward)
				if depth <= 5 {
					return
				}
				sx := (dot(delta, right)/(depth*tan*controller.shownAspect) + 1) * width / 2
				sy := (1 - dot(delta, up)/(depth*tan)) * height / 2
				minX, maxX, minY, maxY = min(minX, sx), max(maxX, sx), min(minY, sy), max(maxY, sy)
			}
		}
		x, y := maxX+16, (minY+maxY)/2-18
		if x+flipControlWidth > width-24 {
			x, y = (minX+maxX)/2-flipControlWidth/2, maxY+14
		}
		// Latch the opening pose, not the changing projected silhouette. Flips
		// keep the same target under the pointer; only reopening/resize reanchors.
		a.turnAnchor = turnControlAnchor{cell: controller.selected, width: width, height: height,
			x: min(max(x, 24), width-flipControlWidth-24), y: min(max(y, 110), height-160), valid: true}
		return
	}
}
