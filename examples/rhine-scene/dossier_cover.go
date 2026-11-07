//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import "github.com/caelyreth/evil-mygo/ui"

// The archive cover is deliberately not a reduced detail screen: identity,
// material and custody remain legible at the browsing camera's oblique angle.
func (d *fileDossier) cover(c *ui.Context) {
	open := float32(d.detail)
	layers := dossierLayersAt(d.detail)
	ui.Box(c.Key("dossier-cover")).Absolute().Fill().IgnoreInput(true).AccessHidden(true).
		Opacity(layers.cover).Translate(-22*open, 0).Blur(2 * open).Children(func() {
		switch d.variant() {
		case 0:
			d.coverLaminate(c, open)
		case 1:
			d.coverSection(c, open)
		case 2:
			d.coverAssay(c)
		case 3:
			d.coverCertificate(c, open)
		}
		ui.Row(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Bottom(dossierInsetBottom).Height(24).
			AlignItems(ui.Center).Children(func() {
			dossierCaption(c, "REFERENCE MATERIAL / SIMULATED RECORD")
			ui.Spacer(c)
			ui.Box(c).Size(154, 20).Draw(func(p *ui.Painter, r ui.Rect) {
				for x, bar := float32(0), 0; x < r.W; bar++ {
					width := float32(1 + (d.cell.number()+bar*7)%3)
					p.Fill(ui.Rect{X: r.X + x, Y: r.Y, W: width, H: r.H}, d.theme.Text, 0)
					x += width + 3
				}
			})
		})
	})
}

func (d *fileDossier) materialFamily() string {
	return d.design().material
}

func (d *fileDossier) coverLaminate(c *ui.Context, open float32) {
	ui.Column(c).Absolute().Left(dossierInsetSide).Top(393).Width(566).Gap(10).Children(func() {
		dossierCaption(c, "MATERIAL")
		ui.Text(c, d.materialFamily()).Font(archiveDisplayFont).FontSize(31).FontWeight(800).LetterSpacing(.5)
		ui.Text(c, "Bonded optical laminate. Retained for transmission, surface quality and structural review.").
			FontSize(17).LineHeight(1.5).TextColor(d.theme.TextMuted)
	})
	d.graphic(c, 0).Absolute().Right(dossierInsetSide+12).Top(247).Size(198, 230).Translate(36*open, -12*open)
	dossierCaption(c, "BONDED SECTION / 3 LAYERS").Absolute().Right(dossierInsetSide).Top(488)
	ui.Row(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(538).Height(99).Gap(28).Children(func() {
		d.coverArchiveBlock(c).Width(344).FillHeight()
		for _, field := range [][2]string{{"BATCH", d.batch()}, {"CUSTODY", "Materials division"}} {
			ui.Column(c.Key(field[0])).Grow(1).Padding(17, 0).Gap(11).Children(func() {
				dossierCaption(c, field[0])
				ui.Text(c, field[1]).FontSize(17).FontWeight(600)
			})
		}
	})
}

func (d *fileDossier) coverArchiveBlock(c *ui.Context) ui.Element {
	return ui.Column(c).Padding(17, 20).Gap(9).Background(panelGreen).Children(func() {
		ui.Text(c, "LABORATORY ARCHIVE").Font("Menlo").FontSize(11).LetterSpacing(1.2).TextColor(panelPaper.Alpha(.8))
		status := "Retained specimen"
		if d.record().reviewed {
			status = "Review complete"
		}
		ui.Text(c, status).FontSize(24).FontWeight(700).TextColor(panelPaper)
	})
}

func (d *fileDossier) coverSection(c *ui.Context, open float32) {
	d.graphic(c, 1).Absolute().Right(dossierInsetSide).Top(236).Size(220, 220).Translate(28*open, 0)
	ui.Column(c).Absolute().Left(dossierInsetSide).Top(399).Width(518).Gap(16).Children(func() {
		dossierCaption(c, "SURFACE / BONDED SECTION")
		ui.Text(c, d.materialFamily()).FontSize(40).FontWeight(800).LetterSpacing(.5)
		ui.Text(c, "Section geometry, surface diffusion and optical continuity.").FontSize(18).
			LineHeight(1.5).TextColor(d.theme.TextMuted)
		ui.Row(c).FillWidth().Height(76).Gap(32).Children(func() {
			for _, field := range [][2]string{{"THICKNESS", "0.42 mm"}, {"LAYERS", "03"}, {"BATCH", d.batch()}} {
				ui.Column(c).Grow(1).Gap(9).Children(func() {
					dossierCaption(c, field[0])
					ui.Text(c, field[1]).Font("Menlo").FontSize(22)
				})
			}
		})
	})
	d.coverArchiveBlock(c).Absolute().Right(dossierInsetSide).Top(490).Size(288, 147)
}

func (d *fileDossier) coverAssay(c *ui.Context) {
	ui.Box(c).Absolute().Left(dossierInsetSide).Top(375).Size(528, 5).Background(panelAccent)
	d.graphic(c, 2).Absolute().Left(dossierInsetSide).Top(402).Size(528, 217)
	ui.Column(c).Absolute().Right(dossierInsetSide).Top(375).Size(340, 262).Padding(24).Gap(14).
		Background(panelGreen).TextColor(panelPaper).Children(func() {
		ui.Text(c, "VISIBLE SPECTRUM").Font("Menlo").FontSize(11).LetterSpacing(1).TextColor(panelPaper.Alpha(.7))
		ui.Text(c, "400-700").Font("Menlo").FontSize(46).LetterSpacing(1).SingleLine()
		ui.Text(c, "nanometers").FontSize(17).TextColor(panelPaper.Alpha(.8))
		ui.Box(c).FillWidth().Height(1).Background(panelPaper.Alpha(.3))
		ui.Text(c, d.materialFamily()).FontSize(24).FontWeight(700)
		ui.Text(c, d.batch()).Font("Menlo").FontSize(16).TextColor(panelPaper.Alpha(.8))
	})
	dossierCaption(c, "SPATIAL TRANSMISSION / REFERENCE MAP").Absolute().Left(dossierInsetSide).Top(629)
}

func (d *fileDossier) coverCertificate(c *ui.Context, open float32) {
	d.graphic(c, 3).Absolute().Right(dossierInsetSide).Top(234).Size(210, 196).Translate(28*open, 0)
	ui.Row(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(420).Height(217).Gap(18).Children(func() {
		ui.Column(c).Width(276).FillHeight().Padding(21).Gap(14).Background(d.theme.Surface).Children(func() {
			ui.Box(c).Size(28, 5).Background(panelRust)
			dossierCaption(c, "MATERIAL")
			ui.Text(c, d.materialFamily()).FontSize(28).FontWeight(800).LineHeight(1.1)
			ui.Text(c, "Production reference").FontSize(16).TextColor(d.theme.TextMuted)
		})
		ui.Column(c).Width(276).FillHeight().Padding(21).Gap(18).Children(func() {
			dossierCaption(c, "BATCH REGISTER")
			ui.Text(c, d.batch()).Font("Menlo").FontSize(24)
			dossierCaption(c, "CONDITIONING")
			ui.Text(c, "23 C\n50% RH").FontSize(23).LineHeight(1.35)
		})
		d.coverArchiveBlock(c).Grow(1).FillHeight()
	})
}
