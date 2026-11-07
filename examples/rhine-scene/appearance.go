//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import "github.com/caelyreth/evil-mygo/ui"

func applyArchiveAppearance(t *ui.Theme, night float32) {
	t.Dark = night >= .5
	t.Background = ui.Hex("#eae5e1").Mix(ui.Hex("#0e1420"), night)
	t.Text = panelInk.Mix(panelPaper, night)
	t.TextMuted = t.Text.Mix(t.Background, .4)
	t.Surface = panelPaper.Mix(ui.Hex("#202c36"), night)
	t.SurfaceHover = panelGreen.Mix(t.Surface, .86)
	t.SurfacePressed = panelGreen.Mix(t.Surface, .74)
	t.Border = t.Text.Alpha(.18)
	t.Accent, t.AccentHover, t.AccentPressed = panelGreen, panelGreen.Mix(panelInk, .16), panelRust
	t.AccentText, t.Focus, t.InactiveAccent = panelPaper, panelGreen, panelGreen
	t.Selection = panelGreen.Mix(panelAccent, night).Alpha(.25)
	t.Inverse, t.InverseText = t.Text, t.Background
	t.Scrollbar = t.Text.Alpha(.3)
}

func (d *fileDossier) setAppearance(night float32) {
	if d.night == night {
		return
	}
	d.night = night
	applyArchiveAppearance(d.theme, night)
	d.theme.Background = panelPaper.Mix(ui.Hex("#ffffff"), .6).Mix(ui.Hex("#16212c"), night)
	d.theme.Surface = panelPaper.Mix(d.theme.Background, .6).Mix(ui.Hex("#22333b"), night)
	d.theme.TextMuted = d.theme.Text.Mix(d.theme.Background, .36)
	d.theme.Accent = panelGreen.Mix(panelPaper, .42*night)
	d.theme.AccentText = panelPaper.Mix(panelInk, night)
	d.theme.Selection = panelGreen.Mix(panelAccent, night).Alpha(.25)
	d.layer.Invalidate()
}
