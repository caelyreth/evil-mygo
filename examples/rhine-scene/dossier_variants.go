//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

type dossierDesign struct{ title, division, material string }

var dossierDesigns = [...]dossierDesign{
	{"Laminate qualification", "STRUCTURAL MATERIALS", "Optical polymer"},
	{"Surface inspection", "MATERIAL ENGINEERING", "Barrier composite"},
	{"Transmission assay", "OPTICAL LABORATORY", "Ceramic laminate"},
	{"Process certification", "PRODUCTION QUALITY", "Coated glass"},
}

func (d *fileDossier) variant() int          { return int(d.cell.dossierHash() & 3) }
func (d *fileDossier) design() dossierDesign { return dossierDesigns[d.variant()] }

func (d *fileDossier) summaryLayout(c *ui.Context, r *laboratoryRecord) {
	switch d.variant() {
	case 0:
		d.summary(c, r)
	case 1:
		d.sectionSummary(c, r)
	case 2:
		d.spectrumSummary(c, r)
	case 3:
		d.ledgerSummary(c, r)
	}
}

func (d *fileDossier) batch() string {
	return fmt.Sprintf("RM-%03d / %02d", 100+d.cell.number()%900, 1+d.cell.number()%12)
}

func (d *fileDossier) profileFields(c *ui.Context) {
	for _, field := range [][2]string{
		{"MATERIAL", d.materialFamily()}, {"BATCH", d.batch()},
		{"CONDITIONING", "23 C / 50% RH"}, {"METHOD", "Spectral transmission"},
	} {
		ui.Column(c.Key(field[0])).Gap(5).Children(func() {
			dossierCaption(c, field[0])
			ui.Text(c, field[1]).FontSize(16).FontWeight(600)
		})
	}
}

func (d *fileDossier) sectionSummary(c *ui.Context, r *laboratoryRecord) {
	ui.Row(c).Fill().Gap(24).Children(func() {
		ui.Column(c).Width(310).Shrink(0).FillHeight().Padding(20).Gap(12).Background(d.theme.Surface).Children(func() {
			dossierCaption(c, "SPECIMEN SECTION")
			d.graphic(c, 1).FillWidth().Grow(1).MinHeight(120)
			ui.Text(c, "Bonded surface / retained sample").FontSize(14).TextColor(d.theme.TextMuted)
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Gap(18).Children(func() {
			ui.Text(c, "Section register").FontSize(25).FontWeight(800)
			ui.Row(c).FillWidth().Gap(20).Children(func() {
				dossierMetric(c, "Transmission", fmt.Sprintf("%.1f", d.transmission(r, r.wavelength, true)), "%")
				dossierMetric(c, "Thickness", "0.42", "mm")
			})
			ui.Row(c).FillWidth().Height(3).Children(func() {
				for _, color := range []ui.Color{panelGreen, panelAccent, panelRust} {
					ui.Box(c).Grow(1).FillHeight().Background(color)
				}
			})
			ui.Row(c).FillWidth().Gap(24).Children(func() {
				ui.Column(c).Grow(1).Gap(20).Children(func() {
					dossierCaption(c, "MATERIAL")
					ui.Text(c, d.materialFamily()).FontSize(19).FontWeight(600)
					dossierCaption(c, "BATCH")
					ui.Text(c, d.batch()).Font("Menlo").FontSize(17)
				})
				ui.Column(c).Grow(1).Gap(20).Children(func() {
					dossierCaption(c, "CONDITIONING")
					ui.Text(c, "23 C / 50% RH").FontSize(19)
					dossierCaption(c, "BONDING")
					ui.Text(c, "Three-layer laminate").FontSize(17)
				})
			})
		})
	})
}

func (d *fileDossier) spectrumSummary(c *ui.Context, r *laboratoryRecord) {
	ui.Column(c).Fill().Gap(18).Children(func() {
		ui.Row(c).FillWidth().Height(94).Gap(24).Children(func() {
			ui.Column(c).Width(280).FillHeight().Padding(14, 20).Gap(4).Background(panelGreen).Children(func() {
				ui.Text(c, "TRANSMISSION").Font("Menlo").FontSize(11).LetterSpacing(1).TextColor(panelPaper.Alpha(.8))
				ui.Textf(c, "%.1f %%", d.transmission(r, r.wavelength, true)).Font("Menlo").FontSize(36).
					TextColor(panelPaper).ContentTransition(archiveHUDText)
			})
			dossierMetric(c, "Probe", fmt.Sprintf("%.0f", r.wavelength), "nm")
			dossierMetric(c, "Haze", fmt.Sprintf("%.1f", 4.2+float64(d.cell.number()%17)*.12), "%")
			dossierMetric(c, "Laminate", "0.42", "mm")
		})
		ui.Row(c).Grow(1).FillWidth().MinHeight(0).Gap(24).Children(func() {
			d.plot(c, r).Grow(1).FillHeight().MinWidth(0)
			ui.Column(c).Width(236).FillHeight().Gap(18).Padding(8, 0).Children(func() {
				dossierCaption(c, "SAMPLE IDENTITY")
				ui.Text(c, d.materialFamily()).FontSize(22).FontWeight(700)
				dossierCaption(c, "BATCH")
				ui.Text(c, d.batch()).Font("Menlo").FontSize(17)
				ui.Text(c, "Visible spectrum\n400-700 nanometers").FontSize(16).LineHeight(1.5).TextColor(d.theme.TextMuted)
			})
		})
	})
}

func (d *fileDossier) ledgerSummary(c *ui.Context, r *laboratoryRecord) {
	ui.Row(c).Fill().Gap(24).Children(func() {
		ui.Column(c).Width(264).Shrink(0).FillHeight().Padding(20).Gap(16).Background(d.theme.Surface).Children(func() {
			ui.Box(c).Size(32, 5).Background(panelRust)
			d.profileFields(c)
		})
		ui.Column(c).Grow(1).FillHeight().Gap(10).Children(func() {
			for i, metric := range [][3]string{
				{"Optical transmission", fmt.Sprintf("%.1f", d.transmission(r, r.wavelength, true)), "%"},
				{"Surface haze", fmt.Sprintf("%.1f", 4.2+float64(d.cell.number()%17)*.12), "%"},
				{"Bonded thickness", "0.42", "mm"},
			} {
				color := [3]ui.Color{panelGreen, panelAccent, panelRust}[i]
				ui.Row(c.Key(metric[0])).Grow(1).FillWidth().AlignItems(ui.Center).Gap(18).Background(d.theme.Surface).Children(func() {
					ui.Box(c).Width(5).FillHeight().Background(color)
					ui.Column(c).Grow(1).Gap(8).Children(func() {
						ui.Text(c, metric[0]).FontSize(17).FontWeight(700)
						dossierCaption(c, []string{"VISIBLE SPECTRUM", "DIFFUSE SCATTER", "THREE BONDED LAYERS"}[i])
					})
					ui.Text(c, metric[1]).Font("Menlo").FontSize(36).TextColor(d.theme.Text).
						ContentTransition(archiveHUDText)
					ui.Text(c, metric[2]).Width(38).FontSize(14).TextColor(d.theme.TextMuted)
				})
			}
		})
	})
}

// Each record family has a different technical silhouette, all drawn natively.
func (d *fileDossier) graphic(c *ui.Context, kind int) ui.Element {
	return ui.Box(c.Key(fmt.Sprintf("specimen-graphic-%d", kind))).Clip().IgnoreInput(true).Draw(func(p *ui.Painter, r ui.Rect) {
		if r.W <= 4 || r.H <= 4 {
			return
		}
		ink := d.theme.Text
		x, y := r.X+r.W/2, r.Y+r.H/2
		s := min(r.W, r.H) * .4
		switch kind {
		case 0:
			for layer, color := range [3]ui.Color{ink, panelAccent, panelGreen} {
				y := r.Y + r.H*.53 - float32(layer)*r.H*.17
				path := new(ui.Path).MoveTo(r.X+r.W*.06, y+r.H*.17).LineTo(r.X+r.W*.63, y).
					LineTo(r.X+r.W*.94, y+r.H*.18).LineTo(r.X+r.W*.36, y+r.H*.35).Close()
				p.FillPath(path, color.Alpha(.14))
				p.StrokePath(path, 1.5, color.Alpha(.7))
			}
		case 1:
			for i, radius := range []float32{s, s * .74, s * .5} {
				color := [3]ui.Color{ink, panelGreen, panelAccent}[i]
				p.Fill(ui.Rect{X: x - radius, Y: y - radius, W: 2 * radius, H: 2 * radius}, color.Alpha(.7), radius)
				p.Fill(ui.Rect{X: x - radius + 2, Y: y - radius + 2, W: 2*radius - 4, H: 2*radius - 4}, d.theme.Surface, radius-2)
			}
			for i := range 24 {
				angle := float64(i) * math.Pi / 12
				dx, dy := float32(math.Cos(angle)), float32(math.Sin(angle))
				p.Line(x+dx*s*.9, y+dy*s*.9, x+dx*s*1.08, y+dy*s*1.08, 1, ink.Alpha(.5))
			}
			p.Line(x-s*1.18, y, x+s*1.18, y, 1, panelRust.Alpha(.65))
			p.Line(x, y-s*1.18, x, y+s*1.18, 1, panelRust.Alpha(.65))
		case 2:
			for row := range 8 {
				for col := range 10 {
					value := .16 + .72*float32(math.Exp(-math.Pow((float64(col)-5)/3, 2)-math.Pow((float64(row)-3)/3, 2)))
					color := panelGreen.Mix(panelAccent, value)
					p.Fill(ui.Rect{X: r.X + float32(col)*r.W/10 + 2, Y: r.Y + float32(row)*r.H/8 + 2,
						W: r.W/10 - 4, H: r.H/8 - 4}, color.Alpha(.2+value*.6), 0)
				}
			}
		case 3:
			path := new(ui.Path).MoveTo(x-s, y-s*.78).LineTo(x+s*.55, y-s*.78).
				LineTo(x+s, y-s*.33).LineTo(x+s, y+s*.78).LineTo(x-s*.55, y+s*.78).
				LineTo(x-s, y+s*.33).Close()
			p.FillPath(path, panelAccent.Alpha(.18))
			p.StrokePath(path, 2, ink.Alpha(.65))
			for i := range 6 {
				line := y - s*.48 + float32(i)*s*.2
				p.Line(x-s*.68, line, x+s*.6, line, 1, ink.Alpha(.22))
			}
			p.Line(x-s*.65, y+s*.32, x-s*.12, y+s*.52, 4, panelGreen)
			p.Line(x-s*.12, y+s*.52, x+s*.6, y-s*.24, 4, panelGreen)
		}
	})
}
