//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const archiveDisplayFont = "Helvetica Neue, Avenir Next"

type archiveApp struct {
	canvas                        *ui.WebGPUCanvas
	renderer                      *sceneRenderer
	controller                    *sceneController
	theme                         *ui.Theme
	dossier                       *fileDossier
	reverse                       *fileDossier
	pages                         *ui.UILayer
	presentation                  dossierPresentation
	surface                       *fileSurface
	reverseSurface                *fileSurface
	viewportHost                  ui.Handle
	width, height                 float32
	hover                         archiveCell
	hasHover                      bool
	press                         archiveCell
	pressed                       bool
	pressReturn                   bool
	pressX, pressY                float32
	dragged                       bool
	lastClick                     archiveCell
	lastClickX, lastClickY        float32
	hasClick                      bool
	wheel                         float32
	readout                       archiveCell
	fileDown, columnDown, rowDown bool
	turnAnchor                    turnControlAnchor
}

func newArchiveApp(canvas *ui.WebGPUCanvas, renderer *sceneRenderer) *archiveApp {
	theme := *ui.LightTheme()
	theme.Font, theme.FontSize, theme.Radius = archiveDisplayFont, 12, 0
	theme.Background = ui.Hex("#eae5e1")
	theme.Surface, theme.SurfaceHover, theme.SurfacePressed = panelPaper, panelGreen.Mix(panelPaper, .86), panelGreen.Mix(panelPaper, .74)
	theme.Border, theme.Text, theme.TextMuted = panelInk, panelInk, panelInk.Mix(panelPaper, .4)
	theme.Accent, theme.AccentHover, theme.AccentPressed = panelGreen, panelGreen.Mix(panelInk, .16), panelRust
	theme.AccentText, theme.Focus = panelPaper, panelGreen
	theme.InactiveAccent = theme.Accent
	a := &archiveApp{canvas: canvas, renderer: renderer, controller: renderer.controller, theme: &theme,
		readout: renderer.controller.selected}
	a.dossier = newFileDossier(&theme, a.controller.selected)
	a.reverse = newBackDossier(&theme, a.controller.selected)
	a.presentation.cell, a.presentation.target = a.controller.selected, a.controller.selected
	a.surface = &fileSurface{}
	a.reverseSurface = &fileSurface{back: true}
	// A native image atlas packs two independently rendered/input-owned layers
	// into WebGPUCanvas's single source. Every capture stays on MyGo's GPU.
	a.pages = ui.NewUILayer(func(c *ui.Context) {
		ui.Box(c).Fill().IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Image(a.dossier.layer.Bitmap(), ui.Rect{X: r.X, Y: r.Y, W: r.W / 2, H: r.H}, ui.FillBox)
			p.Image(a.reverse.layer.Bitmap(), ui.Rect{X: r.X + r.W/2, Y: r.Y, W: r.W / 2, H: r.H}, ui.FillBox)
		})
	}, dossierWidth*2, dossierHeight)
	canvas.SetSource(a.pages.Bitmap())
	return a
}

func (a *archiveApp) command(fn func()) {
	wasBack := a.controller.back
	fn()
	if a.dossier.cell != a.controller.selected || !a.controller.inspectingNow() || wasBack != a.controller.back {
		for _, page := range []*fileDossier{a.dossier, a.reverse} {
			page.layer.HandleSurfaceInput(nil, ui.InputEvent{Kind: ui.InputPointerUp, Button: 0, Canceled: true})
			page.blur()
		}
	}
	if a.controller.dirty {
		a.canvas.Invalidate()
	}
}

func (a *archiveApp) view(c *ui.Context) {
	width, height := c.Size()
	hidden := c.WindowHidden()
	if a.controller.advance(c.Now(), hidden, a.renderer.draws > 0, width/max(height, 1)) {
		a.canvas.Invalidate()
		a.controller.dirty = false
	}
	applyArchiveAppearance(a.theme, float32(a.controller.night.value))
	a.dossier.setAppearance(float32(a.controller.night.value))
	a.reverse.setAppearance(float32(a.controller.night.value))
	c.SetTheme(a.theme)
	if cell := a.controller.selected; cell != a.readout {
		a.fileDown = cell.number() < a.readout.number()
		a.columnDown, a.rowDown = cell.lane < a.readout.lane, cell.row < a.readout.row
		a.readout = cell
	}
	ready := a.renderer.draws > 0 && (a.controller.interactive || sceneTime(a.controller.clock.elapsed) >= 25.58)
	card := a.controller.cards[a.controller.selected]
	// Keep the cover until the file has completed three quarters of its lift.
	detailsReady := a.controller.inspectingNow() && card != nil && card.lift.value >= .4+3.65*.75
	if a.presentation.advance(c.Now(), hidden, ready, c.Preferences().ReduceMotion, a.controller.selected, detailsReady) {
		a.canvas.Invalidate()
	}
	a.dossier.selectCell(a.presentation.cell)
	a.reverse.selectCell(a.presentation.cell)
	inputReady := a.presentation.interactive() && a.presentation.cell == a.controller.selected &&
		a.controller.inspectingNow() && a.controller.detail.value > .85 && !a.controller.turningFile()
	a.dossier.setPresentation(a.presentation.phase, a.presentation.open.value, inputReady && !a.controller.back)
	a.reverse.setPresentation(a.presentation.phase, a.presentation.open.value, inputReady && a.controller.back)
	a.renderer.dossierCell, a.renderer.dossierVisible = a.presentation.cell, a.presentation.phase > 0
	a.renderer.dossierOpacity = float32(dossierCoverage(a.presentation.phase))
	a.renderPages(min(max(c.PixelsPerDIP(), 1), 2))
	pixelWidth := max(int(math.Ceil(float64(width*c.PixelsPerDIP()))), 1)
	pixelHeight := max(int(math.Ceil(float64(height*c.PixelsPerDIP()))), 1)
	a.renderer.prepare(float32(pixelWidth) / float32(pixelHeight))
	a.surface.update(a.controller, width, height, inputReady && !a.controller.back)
	a.reverseSurface.update(a.controller, width, height, inputReady && a.controller.back)
	inputPage, inputSurface := a.inputPage()
	// Register actions, not polling: native controls retain keyboard focus and
	// consume their own Enter/Space before the window shortcuts see them.
	if inputPage == nil || !a.viewportHost.Focused(c) {
		c.OnWindowShortcut(0, ui.KeyUp, func() { a.command(func() { a.controller.navigate(0, -1) }) })
		c.OnWindowShortcut(0, ui.KeyDown, func() { a.command(func() { a.controller.navigate(0, 1) }) })
		c.OnWindowShortcut(0, ui.KeyLeft, func() { a.command(func() { a.controller.navigate(-1, 0) }) })
		c.OnWindowShortcut(0, ui.KeyRight, func() { a.command(func() { a.controller.navigate(1, 0) }) })
		c.OnWindowShortcut(0, ui.KeyEnter, func() {
			if !c.ShortcutRepeated() {
				a.command(func() { a.controller.inspect(!a.controller.inspectingNow()) })
			}
		})
	}
	c.OnWindowShortcut(0, ui.KeyEscape, func() { a.command(func() { a.controller.inspect(false) }) })
	ui.Box(c.Key("archive-root")).Fill().Background(a.theme.Background).Children(func() {
		viewport := ui.Box(c.Key("gpu-viewport")).Absolute().Fill().Focusable().AutoFocus().FocusRing(false).Bind(&a.viewportHost).
			Label("Archive scene").Description("Click a card to select it. Double-click to inspect. Arrow keys navigate; Escape returns.").
			HandleInput(func(ev ui.InputEvent) bool { return a.viewportInput(c, ev) })
		if a.hasHover {
			viewport.Cursor(ui.CursorPointer)
		}
		if inputPage != nil {
			viewport.HostLayer(inputPage.layer, inputSurface)
		}
		viewport.Draw(func(p *ui.Painter, r ui.Rect) {
			if r.W <= 0 || r.H <= 0 {
				return
			}
			a.width, a.height = r.W, r.H
			a.canvas.Resize(int(math.Ceil(float64(r.W*p.Scale()))), int(math.Ceil(float64(r.H*p.Scale()))))
			p.Image(a.canvas.Bitmap(), r, ui.FillBox)
		})
		a.frame(c)
		a.header(c, width, height)
		a.footer(c, width)
		a.turnControl(c, width, height)
	})
	if !hidden && (a.controller.moving() || a.presentation.moving() || a.dossier.layer.NeedsFrame() || a.reverse.layer.NeedsFrame() || a.pages.NeedsFrame()) {
		c.AnimationFrame()
	}
}

func (a *archiveApp) close() {
	a.pages.Close()
	a.dossier.layer.Close()
	a.reverse.layer.Close()
}

func (a *archiveApp) renderPages(scale float32) {
	for _, page := range []*fileDossier{a.dossier, a.reverse} {
		page.layer.SetScale(scale)
		if page.layer.NeedsFrame() {
			page.layer.Render()
			a.pages.Invalidate()
		}
	}
	a.pages.SetScale(scale)
	if a.pages.NeedsFrame() {
		a.pages.Render()
		a.canvas.Invalidate()
	}
}

func (a *archiveApp) inputPage() (*fileDossier, *fileSurface) {
	if a.surface.enabled {
		return a.dossier, a.surface
	}
	if a.reverseSurface.enabled {
		return a.reverse, a.reverseSurface
	}
	return nil, nil
}

func (a *archiveApp) viewportInput(c *ui.Context, ev ui.InputEvent) bool {
	page, surface := a.inputPage()
	if ev.Kind == ui.InputPointerDown && ev.Button == 0 && a.controller.inspectingNow() {
		cell, hit := a.controller.pick(ev.X, ev.Y, a.width, a.height)
		if !hit || cell != a.controller.selected {
			// A layer's hover-leave notification must not swallow the outside
			// press that closes an examined file, on either face.
			if page != nil {
				page.layer.HandleSurfaceInput(surface, ui.InputEvent{Kind: ui.InputPointerLeave})
			}
			return a.input(ev)
		}
	}
	if a.pressed && a.pressReturn && (ev.Kind == ui.InputPointerUp || ev.Kind == ui.InputPointerMove) {
		return a.input(ev)
	}
	if page != nil && surface.cell == a.controller.selected {
		// As in layer-membrane: the framework maps UVs, captures native controls,
		// and owns their focus/IME. Only unclaimed gestures reach the archive.
		taken := a.viewportHost.HandleLayerInput(c, ev)
		if page.layer.NeedsFrame() {
			c.Invalidate()
		}
		if ev.Kind == ui.InputScroll {
			_, overFile := surface.ScreenToSurface(ui.Point{X: ev.X, Y: ev.Y})
			taken = taken || overFile
		}
		if taken || ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyTab {
			return true
		}
	}
	return a.input(ev)
}

func (a *archiveApp) input(ev ui.InputEvent) bool {
	switch ev.Kind {
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		a.press, a.pressed = a.controller.pick(ev.X, ev.Y, a.width, a.height)
		a.pressReturn = a.controller.inspectingNow() && (!a.pressed || a.press != a.controller.selected)
		a.pressed = a.pressed || a.pressReturn
		dx, dy := ev.X-a.lastClickX, ev.Y-a.lastClickY
		if !a.pressReturn && ev.Clicks >= 2 && a.hasClick && dx*dx+dy*dy <= 25 {
			// The first click may already be moving the card and camera.
			a.press, a.pressed = a.lastClick, true
		}
		a.pressX, a.pressY, a.dragged = ev.X, ev.Y, false
		return true
	case ui.InputPointerMove:
		if ev.Button == 0 {
			dx, dy := ev.X-a.pressX, ev.Y-a.pressY
			a.dragged = a.dragged || dx*dx+dy*dy > 25
			return true
		}
		cell, hit := a.controller.pick(ev.X, ev.Y, a.width, a.height)
		changed := hit != a.hasHover || hit && cell != a.hover
		a.hover, a.hasHover = cell, hit
		return changed
	case ui.InputPointerLeave:
		changed := a.hasHover
		a.hasHover = false
		return changed
	case ui.InputPointerUp:
		if ev.Button != 0 {
			return false
		}
		defer func() { a.pressed, a.pressReturn = false, false }()
		if ev.Canceled || a.dragged || !a.pressed || ev.X < 0 || ev.Y < 0 || ev.X >= a.width || ev.Y >= a.height {
			return true
		}
		if a.pressReturn {
			a.command(func() { a.controller.inspect(false) })
			a.hasClick = false
			return true
		}
		cell := a.press
		dx, dy := ev.X-a.lastClickX, ev.Y-a.lastClickY
		double := ev.Clicks >= 2 && a.hasClick && dx*dx+dy*dy <= 25
		if double {
			cell = a.lastClick
		}
		a.command(func() {
			a.controller.selectCell(cell)
			if double {
				a.controller.inspect(true)
			}
		})
		a.lastClick, a.lastClickX, a.lastClickY, a.hasClick = cell, ev.X, ev.Y, true
		return true
	case ui.InputScroll:
		if ev.DY == 0 && ev.DX == 0 {
			return false
		}
		delta := ev.DY
		lane := math.Abs(float64(ev.DX)) > math.Abs(float64(ev.DY))
		if lane {
			delta = ev.DX
		}
		a.wheel += delta
		if !ev.Precise || math.Abs(float64(a.wheel)) >= 40 {
			step := 1
			if a.wheel < 0 {
				step = -1
			}
			a.command(func() {
				if lane {
					a.controller.navigate(step, 0)
				} else {
					a.controller.navigate(0, step)
				}
			})
			a.wheel = 0
		}
		return true
	}
	return false
}

var (
	panelInk    = ui.Hex("#141c2c")
	panelPaper  = ui.Hex("#f4e0bc")
	panelAccent = ui.Hex("#dca854")
	panelGreen  = ui.Hex("#3f5b4b")
	panelRust   = ui.Hex("#8c3c38")
)

func machineLabel(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).Font("Menlo").FontSize(9).LetterSpacing(.7).
		ContentTransition(archiveHUDText)
}

func (a *archiveApp) frame(c *ui.Context) {
	bar := c.TitleBar()
	titleBarHeight := max(bar.Height, 48)
	ui.Box(c.Key("instrument-frame")).Absolute().Fill().IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
		// Print-registration details stay at the edges, not around the scene.
		p.Line(34, r.H-25, 44, r.H-25, 2, a.theme.Text)
		p.Line(39, r.H-30, 39, r.H-20, 2, a.theme.Text)
		paintHUDDither(p, ui.Rect{X: r.W - 108, Y: r.H - 29, W: 74, H: 10}, a.theme.Text.Alpha(.35))
	})
	// The GPU viewport already fills the window, including behind the native
	// controls. Keep only an invisible drag handle here, not a separate header.
	ui.Box(c.Key("window-drag-region")).Absolute().Left(bar.Left).Right(bar.Right).Top(0).
		Height(titleBarHeight).Background(ui.Transparent).DragWindow()
}

func (a *archiveApp) header(c *ui.Context, width, height float32) {
	short := height < 650
	top := max(c.TitleBar().Height, 48) + 6
	titleSize, titleWidth, logoWidth, recordWidth := float32(42), float32(350), float32(72), float32(192)
	if width < 950 {
		titleSize, titleWidth, logoWidth, recordWidth = 32, 278, 58, 176
	}
	if short {
		titleSize, titleWidth, logoWidth = 28, 244, 50
	}
	ui.Row(c.Key("archive-title")).Absolute().Left(34).Top(top).Width(titleWidth).
		Gap(12).AlignItems(ui.Center).IgnoreInput(true).
		Transition(archiveHUDLayout).
		Draw(func(p *ui.Painter, r ui.Rect) {
			for i, color := range [3]ui.Color{panelInk, panelGreen, panelRust} {
				p.Fill(ui.Rect{X: r.X + logoWidth*float32(i)/3, Y: r.Y + r.H + 8, W: logoWidth / 3, H: 3}, color, 0)
			}
		}).Children(func() {
		ui.Icon(c, rhineLabLogo).Size(logoWidth, logoWidth*145/310).Shrink(0).TextColor(a.theme.Text).
			Label("Rhine Lab emblem")
		ui.Text(c, "RHINE LAB").Font(archiveDisplayFont).FontSize(titleSize).FontWeight(900).
			LetterSpacing(2.5).LineHeight(.95).SingleLine()
	})
	ui.Column(c.Key("record-register")).Absolute().Right(34).Top(top).Width(recordWidth).
		Background(a.theme.Surface).IgnoreInput(true).Transition(archiveHUDLayout).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y + 18, W: recordWidth / 5, H: r.H - 18}, panelAccent, 0)
		paintHUDHalftone(p, ui.Rect{X: r.X + r.W*.55, Y: r.Y + 22, W: r.W*.45 - 4, H: r.H - 28}, a.theme.Text.Alpha(.1))
		for i := 1; i < 5; i++ {
			x := r.X + r.W*float32(i)/5
			p.Line(x, r.Y+26, x, r.Y+r.H-20, .5, a.theme.Text.Alpha(.09))
		}
	}).Children(func() {
		ui.Row(c).FillWidth().Height(18).AlignItems(ui.Center).Background(panelGreen).Children(func() {
			ui.Text(c, "SELECTED FILE").Font("Menlo").FontSize(8).FontWeight(700).
				LetterSpacing(.8).Padding(0, 8).TextColor(panelPaper)
			ui.Spacer(c)
			ui.Box(c).Width(28).FillHeight().Stripes(panelPaper.Alpha(.3), 1, 5, 45)
		})
		ui.Column(c).FillWidth().Padding(6, 0).Gap(6).TextColor(a.theme.Text).Children(func() {
			size := float32(50)
			if width < 950 {
				size = 44
			}
			if short {
				size = 40
			}
			// One equal-width native cell per digit: the gold band is exactly
			// the first cell, so font metrics cannot straddle its edge.
			number := fmt.Sprintf("%05d", a.controller.selected.number())
			ui.Row(c.Key("selected-file-number")).FillWidth().Label("Selected file " + number).Children(func() {
				for i, digit := range number {
					ink := a.theme.Text
					if i == 0 {
						ink = panelInk
					}
					ui.Box(c.Key(fmt.Sprintf("file-digit-%d", i))).Width(recordWidth / 5).Center().Children(func() {
						ui.Text(c, string(digit)).Font(archiveDisplayFont).FontSize(size).FontWeight(900).
							FontFeatures("tnum").TextColor(ink).LineHeight(1).SingleLine().AccessHidden(true).
							ContentTransition(archiveHUDNumber(a.fileDown))
					})
				}
			})
			ui.Row(c).FillWidth().Padding(0, 8).Height(7).Children(func() {
				ui.Box(c).Width(44).FillHeight().Stripes(a.theme.Text.Alpha(.5), 1, 4, 45)
				ui.Spacer(c)
				ui.Box(c).Width(32).FillHeight().Draw(func(p *ui.Painter, r ui.Rect) {
					paintHUDDither(p, r, a.theme.Text.Alpha(.5))
				})
			})
		})
	})
}

func (a *archiveApp) travelButton(c *ui.Context, label string, lane, row int) {
	cell := a.controller.selected
	target := archiveCell{cell.lane + lane, cell.row + row}
	button := ui.ButtonBase(c.Key(label)).Label(label).Role(ui.RoleButton).
		Size(30, 32).Shrink(0).Radius(0).Background(panelInk).Cursor(ui.CursorPointer).
		Disabled(!target.valid())
	face, ink := panelInk, panelPaper
	if !target.valid() {
		face = panelInk.Alpha(.25)
	}
	if button.Hovered() {
		face, ink = panelGreen, panelPaper
	}
	if button.Pressed() {
		face, ink = panelRust, panelPaper
	}
	ink = animateHUDButton(button, face, ink)
	button.Draw(func(p *ui.Painter, r ui.Rect) {
		x, y := r.X+r.W/2, r.Y+r.H/2
		dx, dy := float32(lane), float32(row)
		p.Line(x-dx*8, y-dy*8, x+dx*7, y+dy*7, 2.5, ink)
		p.Line(x+dx*7, y+dy*7, x+dx*1-dy*5, y+dy*1+dx*5, 2.5, ink)
		p.Line(x+dx*7, y+dy*7, x+dx*1+dy*5, y+dy*1-dx*5, 2.5, ink)
	}).OnClick(func() { a.command(func() { a.controller.navigate(lane, row) }) })
}

func (a *archiveApp) travelHUD(c *ui.Context, columns bool) {
	cell := a.controller.selected
	key, unit := "column-drive", "COLUMN"
	position, total := cell.lane-minLane+1, archiveColumns
	negative, positive, lane, row := "Previous column", "Next column", 1, 0
	down := a.columnDown
	if !columns {
		key, unit = "row-drive", "ROW"
		position, total = cell.row-minRow+1, archiveRows
		negative, positive, lane, row = "Previous file", "Next file", 0, 1
		down = a.rowDown
	}
	ui.Row(c.Key(key)).FillWidth().Gap(8).AlignItems(ui.Center).PassThrough().Children(func() {
		ui.Column(c).Width(54).Gap(6).IgnoreInput(true).Children(func() {
			ui.Box(c).Size(14, 3).Background(panelGreen)
			ui.Text(c, unit).Font(archiveDisplayFont).FontSize(9).FontWeight(800).LetterSpacing(.5).TextColor(a.theme.Text)
		})
		a.travelButton(c, negative, -lane, -row)
		ui.Box(c).Size(68, 32).Center().IgnoreInput(true).Children(func() {
			ui.Textf(c.Key("position-number"), "%03d", position).Font(archiveDisplayFont).FontSize(30).
				FontWeight(900).FontFeatures("tnum").LineHeight(1).LetterSpacing(1.5).TextColor(a.theme.Text).SingleLine().FillWidth().TextAlign(ui.Center).
				ContentTransition(archiveHUDNumber(down))
			machineLabel(c, fmt.Sprintf("/ %03d", total)).Absolute().Left(0).Right(0).Top(34).
				FontSize(8).LetterSpacing(0).TextColor(a.theme.TextMuted).TextAlign(ui.Center)
		})
		a.travelButton(c, positive, lane, row)
	})
}

func (a *archiveApp) footer(c *ui.Context, width float32) {
	ui.Column(c.Key("archive-navigation")).Absolute().Left(34).Bottom(44).Width(214).Gap(18).PassThrough().
		Transition(archiveHUDLayout).Children(func() {
		a.travelHUD(c, true)
		a.travelHUD(c, false)
	})
	a.viewControls(c)
	actionWidth := float32(192)
	if width < 950 {
		actionWidth = 160
	}
	text, label, rotation := "EXAMINE", "Inspect file", float32(0)
	face, ink := panelAccent, panelInk
	if a.controller.inspectingNow() {
		text, label, rotation = "RETURN", "Return to archive", 180
		face, ink = panelGreen, panelPaper
	}
	button := ui.ButtonBase(c.Key("inspect")).Label(label).Role(ui.RoleButton).
		Absolute().Left((width-actionWidth)/2).Bottom(44).Width(actionWidth).Height(48).
		Padding(0, 14).Gap(12).Radius(0).Background(face).Cursor(ui.CursorPointer).
		Transition(archiveHUDLayout)
	if button.Hovered() {
		face, ink = panelInk, panelPaper
	}
	if button.Pressed() {
		face, ink = panelRust, panelPaper
	}
	ink = animateHUDButton(button, face, ink)
	button.Draw(func(p *ui.Painter, r ui.Rect) {
		paintHUDDither(p, ui.Rect{X: r.X + r.W - 30, Y: r.Y + 2, W: 28, H: 6}, ink)
	}).Children(func() {
		fontSize := float32(24)
		if width < 950 {
			fontSize = 21
		}
		ui.Text(c.Key("inspection-label"), text).Font(archiveDisplayFont).FontSize(fontSize).FontWeight(900).
			LetterSpacing(.75).TextColor(ink).SingleLine().ContentTransition(archiveHUDText)
		ui.Spacer(c)
		ui.Box(c.Key("inspection-direction")).Size(16, 18).
			AnimatePose(ui.Pose{Rotate: rotation}, archiveHUDMotion).Draw(func(p *ui.Painter, r ui.Rect) {
			x, y := r.X+r.W/2, r.Y+r.H/2
			p.Line(x-7, y, x+6, y, 2.5, ink)
			p.Line(x+6, y, x, y-6, 2.5, ink)
			p.Line(x+6, y, x, y+6, 2.5, ink)
		})
	}).OnClick(func() { a.command(func() { a.controller.inspect(!a.controller.inspectingNow()) }) })
}

func (a *archiveApp) viewControls(c *ui.Context) {
	ui.Column(c.Key("view-controls")).Absolute().Right(34).Bottom(44).Width(184).Gap(10).PassThrough().Children(func() {
		ui.Row(c.Key("camera-presets")).Gap(4).AlignItems(ui.Center).PassThrough().Children(func() {
			machineLabel(c, "VIEW").Width(42).FontSize(8).TextColor(a.theme.TextMuted).IgnoreInput(true)
			a.viewChoice(c, "Near", !a.controller.farView, false, func() { a.controller.setFarView(false) })
			a.viewChoice(c, "Far", a.controller.farView, false, func() { a.controller.setFarView(true) })
		})
		ui.Row(c.Key("lighting-presets")).Gap(4).AlignItems(ui.Center).PassThrough().Children(func() {
			machineLabel(c, "LIGHT").Width(42).FontSize(8).TextColor(a.theme.TextMuted).IgnoreInput(true)
			a.viewChoice(c, "Light", !a.controller.darkMode, true, func() { a.controller.setDarkMode(false) })
			a.viewChoice(c, "Dark", a.controller.darkMode, true, func() { a.controller.setDarkMode(true) })
		})
	})
}

func (a *archiveApp) viewChoice(c *ui.Context, label string, selected, lighting bool, action func()) {
	face, ink := a.theme.Text.Alpha(.07), a.theme.Text
	if selected {
		face, ink = panelGreen, panelPaper
		if lighting {
			face, ink = panelAccent, panelInk
		}
	}
	b := ui.ButtonBase(c.Key(label)).Label(label).Role(ui.RoleButton).Checked(selected).
		Size(64, 32).Shrink(0).Center().Radius(0).Cursor(ui.CursorPointer)
	if b.Hovered() {
		face, ink = panelInk, panelPaper
	}
	if b.Pressed() {
		face, ink = panelRust, panelPaper
	}
	ink = animateHUDButton(b, face, ink)
	b.Children(func() {
		ui.Text(c, label).Font(archiveDisplayFont).FontSize(12).FontWeight(700).LetterSpacing(.4).TextColor(ink)
	}).OnClick(func() { a.command(action) })
}
