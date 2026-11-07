//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import "github.com/egoist/mygo/ui"

type archiveLauncher struct {
	archive *archiveApp
	theme   ui.Theme
}

func newArchiveLauncher() *archiveLauncher {
	theme := *ui.LightTheme()
	theme.Font, theme.FontSize, theme.Radius = archiveDisplayFont, 14, 0
	applyArchiveAppearance(&theme, 0)
	return &archiveLauncher{theme: theme}
}

func (s *archiveLauncher) open() {
	if s.archive != nil {
		return
	}
	// Neither the geometry nor its scene/UI GPU resources are initialized until
	// the explicit action. The entrance clock starts with its first GPU frame.
	renderer := &sceneRenderer{model: newArchiveModel(), controller: newSceneController()}
	canvas := ui.NewWebGPUCanvas(1, 1, renderer.draw)
	s.archive = newArchiveApp(canvas, renderer)
}

func (s *archiveLauncher) view(c *ui.Context) {
	if s.archive != nil {
		s.archive.view(c)
		return
	}
	c.SetTheme(&s.theme)
	width, height := c.Size()
	compact := width < 950 || height < 650
	contentWidth, titleSize, emblemWidth := float32(440), float32(48), float32(110)
	if compact {
		contentWidth, titleSize, emblemWidth = 360, 38, 88
	}
	ui.Box(c.Key("archive-startup")).Fill().Background(s.theme.Background).Children(func() {
		ui.Box(c).Absolute().Fill().IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
			paintHUDHalftone(p, ui.Rect{X: r.W - 260, Y: 68, W: 220, H: 150}, panelInk.Alpha(.09))
			paintHUDDither(p, ui.Rect{X: 34, Y: r.H - 76, W: 96, H: 18}, panelInk.Alpha(.22))
			for i, color := range [3]ui.Color{panelInk, panelGreen, panelRust} {
				p.Fill(ui.Rect{X: r.W - 118 + 28*float32(i), Y: r.H - 42, W: 28, H: 3}, color, 0)
			}
		})
		bar := c.TitleBar()
		ui.Box(c.Key("startup-drag-region")).Absolute().Left(bar.Left).Right(bar.Right).Top(0).
			Height(max(bar.Height, 48)).Background(ui.Transparent).DragWindow()
		ui.Column(c.Key("startup-content")).Absolute().Left(width/2).Top(height/2+12).
			TranslatePercent(-50, -50).Width(contentWidth).Gap(22).Children(func() {
			ui.Icon(c, rhineLabLogo).Size(emblemWidth, emblemWidth*145/310).TextColor(panelInk).Label("Rhine Lab emblem")
			ui.Column(c).Gap(12).Children(func() {
				ui.Text(c, "RHINE LAB").Font(archiveDisplayFont).FontSize(titleSize).FontWeight(900).
					LetterSpacing(3).LineHeight(1).SingleLine()
				machineLabel(c, "MATERIAL ARCHIVE").FontSize(11).LetterSpacing(2).TextColor(panelGreen)
			})
			ui.Row(c).Width(96).Height(3).Children(func() {
				for _, color := range [3]ui.Color{panelInk, panelGreen, panelRust} {
					ui.Box(c).Grow(1).FillHeight().Background(color)
				}
			})
			button := ui.ButtonBase(c.Key("open-archive")).Label("Open archive").Role(ui.RoleButton).
				Focusable().AutoFocus().Width(224).Height(50).Padding(0, 18).Gap(16).Radius(0).Cursor(ui.CursorPointer)
			face, ink := panelAccent, panelInk
			if button.Hovered() {
				face, ink = panelGreen, panelPaper
			}
			if button.Pressed() {
				face, ink = panelRust, panelPaper
			}
			ink = animateHUDButton(button, face, ink)
			button.Children(func() {
				ui.Text(c, "OPEN ARCHIVE").FontSize(16).FontWeight(800).LetterSpacing(1).TextColor(ink)
				ui.Spacer(c)
				ui.Box(c).Size(16, 18).Draw(func(p *ui.Painter, r ui.Rect) {
					x, y := r.X+r.W/2, r.Y+r.H/2
					p.Line(x-7, y, x+6, y, 2, ink)
					p.Line(x+6, y, x+1, y-5, 2, ink)
					p.Line(x+6, y, x+1, y+5, 2, ink)
				})
			}).OnClick(func() {
				s.open()
				c.Invalidate()
			})
		})
	})
}

func (s *archiveLauncher) close() {
	if s.archive != nil {
		s.archive.close()
		s.archive.renderer.release()
		s.archive = nil
	}
}
