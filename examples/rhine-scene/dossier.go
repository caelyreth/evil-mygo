//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	dossierWidth  = 1000
	dossierHeight = 740
	// The top rail clears the model's index inlay and upper fitting; the
	// side/bottom gutters clear both screw heads, including their refraction.
	dossierInsetTop    = 68
	dossierInsetSide   = 52
	dossierInsetBottom = 48
)

type laboratoryRecord struct {
	tab          int
	wavelength   float64
	comparison   bool
	scans        int
	reviewed     bool
	draft, notes string
}

type fileDossier struct {
	layer       *ui.UILayer
	theme       *ui.Theme
	cell        archiveCell
	records     map[archiveCell]*laboratoryRecord
	handling    map[archiveCell]*custodyRecord
	reverse     bool
	blurPending bool
	phase       float64
	detail      float64
	interactive bool
	night       float32
}

func newFileDossier(theme *ui.Theme, cell archiveCell) *fileDossier {
	return newDossier(theme, cell, false)
}

func newBackDossier(theme *ui.Theme, cell archiveCell) *fileDossier {
	return newDossier(theme, cell, true)
}

func newDossier(theme *ui.Theme, cell archiveCell, reverse bool) *fileDossier {
	t := *theme
	t.Font, t.FontSize = "Avenir Next, Helvetica Neue", 16
	t.Background = panelPaper.Mix(ui.Hex("#ffffff"), .6)
	t.Surface, t.Border = panelPaper.Mix(t.Background, .6), panelInk.Alpha(.18)
	t.Selection = panelGreen.Alpha(.22)
	d := &fileDossier{theme: &t, cell: cell, reverse: reverse,
		records: make(map[archiveCell]*laboratoryRecord), handling: make(map[archiveCell]*custodyRecord)}
	d.layer = ui.NewUILayer(d.view, dossierWidth, dossierHeight)
	return d
}

func (d *fileDossier) selectCell(cell archiveCell) {
	if d.cell == cell {
		return
	}
	// Finish pending edits against the old record before changing the keyed tree.
	if d.layer.NeedsFrame() {
		d.layer.Render()
	}
	d.cell = cell
	d.blurPending = true
	d.layer.Invalidate()
}

func (d *fileDossier) blur() {
	d.blurPending = true
	d.layer.Invalidate()
}

func (d *fileDossier) setPresentation(phase, detail float64, interactive bool) {
	detail = min(max(detail, 0), 1)
	d.phase = phase
	if d.detail != detail || d.interactive != interactive {
		d.detail, d.interactive = detail, interactive
		d.layer.Invalidate()
	}
}

func (d *fileDossier) record() *laboratoryRecord {
	r := d.records[d.cell]
	if r == nil {
		r = &laboratoryRecord{wavelength: 550, comparison: true}
		d.records[d.cell] = r
	}
	return r
}

func (d *fileDossier) view(c *ui.Context) {
	c.SetTheme(d.theme)
	ui.Box(c.Key("dossier-surface")).Fill().Clip().Children(func() {
		root := ui.Box(c.Key(fmt.Sprintf("dossier-%05d", d.cell.number()))).Fill().
			Background(d.theme.Background).TextColor(d.theme.Text).Focusable().AutoFocus().FocusRing(false).
			Disabled(!d.interactive).DimDisabled(false)
		if d.blurPending && d.interactive {
			root.Focus()
			d.blurPending = false
		}
		root.Children(func() {
			if d.reverse {
				d.reversePage(c)
			} else {
				d.cover(c)
				d.page(c)
				d.chrome(c)
			}
		})
	})
}

func (d *fileDossier) chrome(c *ui.Context) {
	open := float32(smooth(d.detail / .62))
	ui.Row(c.Key("dossier-letterhead")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).
		Top(dossierInsetTop).Height(32).AlignItems(ui.Center).Gap(12).IgnoreInput(true).Children(func() {
		ui.Icon(c, rhineLabLogo).Size(64, 30).TextColor(d.theme.Text)
		ui.Text(c, "RHINE LAB").Font(archiveDisplayFont).FontWeight(900).FontSize(23).LetterSpacing(2)
		ui.Spacer(c)
		dossierCaption(c, "MATERIALS / QUALITY ASSURANCE")
	})
	ui.Row(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(116).Height(3).IgnoreInput(true).Children(func() {
		for _, color := range [3]ui.Color{panelInk, panelGreen, panelRust} {
			ui.Box(c).Grow(1).FillHeight().Background(color)
		}
	})
	ui.Column(c.Key("dossier-title")).Absolute().Left(dossierInsetSide).Top(145 - 10*open).Width(690).
		Gap(5).IgnoreInput(true).Children(func() {
		dossierCaption(c, d.design().division)
		ui.Text(c, d.design().title).Font(archiveDisplayFont).FontSize(50 - 14*open).FontWeight(800).
			LetterSpacing(.6).LineHeight(1).SingleLine()
	})
	// One specimen register travels from the cover's large index into the
	// detail header. It never disappears or swaps to a duplicate text layer.
	ui.Column(c.Key("dossier-specimen")).Absolute().Left(dossierInsetSide + 676*open).Top(236 - 96*open).
		Width(640 - 420*open).Gap(4).IgnoreInput(true).Children(func() {
		dossierCaption(c, "SPECIMEN")
		ui.Textf(c, "RL-%05d", d.cell.number()).Font("Menlo").FontSize(112 - 86*open).
			LetterSpacing(2 - open).LineHeight(1).SingleLine().TextColor(d.theme.Text)
	})
}

func (d *fileDossier) page(c *ui.Context) {
	r := d.record()
	layers := dossierLayersAt(d.detail)
	ui.Row(c.Key("dossier-tabs")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(221).Height(42).Gap(8).
		Opacity(layers.tabs).Translate(0, 14*(1-layers.tabs)).Blur(2 * (1 - layers.tabs)).Children(func() {
		for tab, name := range []string{"Summary", "Analysis", "Notes"} {
			face, ink := ui.Transparent, d.theme.Text
			if r.tab == tab {
				face, ink = panelGreen, panelPaper
			}
			dossierButton(c, name, face, ink, func() { r.tab = tab }).Width(132).Height(42).Checked(r.tab == tab)
		}
		ui.Spacer(c)
		status, color := "Awaiting analysis", d.theme.Text
		if r.scans > 0 {
			status, color = "Analysis complete", d.theme.Accent
		}
		if r.reviewed {
			status = "Reviewed"
		}
		ui.Box(c).Height(42).Padding(0, 16).Center().Background(color.Alpha(.08)).Children(func() {
			ui.Text(c, status).FontSize(14).FontWeight(700).TextColor(color).ContentTransition(archiveHUDText)
		})
	})
	ui.Column(c.Key("dossier-body")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(279).Bottom(110).
		Opacity(layers.body).Translate(0, 20*(1-layers.body)).ClipInsetPercent(0, 0, 100*(1-layers.body), 0, 0).
		Children(func() {
			switch r.tab {
			case 0:
				d.summaryLayout(c, r)
			case 1:
				d.analysis(c, r)
			case 2:
				d.notes(c, r)
			}
		})
	ui.Row(c.Key("dossier-actions")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Bottom(dossierInsetBottom).
		Height(46).AlignItems(ui.Center).Gap(10).Opacity(layers.footer).Translate(0, 12*(1-layers.footer)).Children(func() {
		dossierCaption(c, "DEMONSTRATION RECORD / SIMULATED ASSAY")
		ui.Spacer(c)
		dossierButton(c, "Run sample analysis", panelAccent, panelInk, func() {
			r.scans++
			r.reviewed = false
		}).Width(214)
		label := "Mark reviewed"
		if r.reviewed {
			label = "Reviewed"
		}
		dossierButton(c, label, panelGreen, panelPaper, func() { r.reviewed = true }).Width(168).
			Disabled(r.scans == 0 || r.reviewed)
	})
	// A measured registration sweep opens the report from its header downward;
	// the same reveal closes upward on Return, without a separate exit effect.
	ui.Box(c.Key("dossier-registration-sweep")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).
		Top(279 + 351*layers.body).Height(10).Opacity(float32(4*d.detail*(1-d.detail)) * .65).
		IgnoreInput(true).AccessHidden(true).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 2}, panelGreen, 0)
		p.Fill(ui.Rect{X: r.X, Y: r.Y + 2, W: r.W, H: 6}, panelAccent.Alpha(.22), 0)
		for x := float32(0); x < r.W; x += 32 {
			p.Line(r.X+x, r.Y-3, r.X+x, r.Y+3, 1, panelGreen)
		}
	})
}

func dossierCaption(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).Font("Menlo").FontSize(11).LetterSpacing(.8).TextColor(c.Theme().TextMuted)
}

func dossierButton(c *ui.Context, label string, face, ink ui.Color, action func()) ui.Element {
	b := ui.ButtonBase(c.Key(label)).Label(label).Role(ui.RoleButton).Height(46).Padding(0, 16).
		Radius(0).Background(face).Cursor(ui.CursorPointer)
	if b.Hovered() {
		face, ink = panelInk, panelPaper
	}
	if b.Pressed() {
		face, ink = panelRust, panelPaper
	}
	ink = animateHUDButton(b, face, ink)
	b.Children(func() {
		ui.Text(c, label).FontSize(15).FontWeight(700).LetterSpacing(.2).TextColor(ink).SingleLine().
			ContentTransition(archiveHUDText)
	}).OnClick(func() {
		action()
		c.Invalidate()
	})
	return b
}

func (d *fileDossier) summary(c *ui.Context, r *laboratoryRecord) {
	ui.Row(c).Fill().Gap(24).Children(func() {
		ui.Column(c).Width(292).Shrink(0).FillHeight().Padding(22).Gap(16).Background(d.theme.Surface).Children(func() {
			ui.Text(c, "Sample profile").FontSize(20).FontWeight(800)
			family := d.materialFamily()
			for _, field := range [][2]string{
				{"Material", family},
				{"Batch", fmt.Sprintf("RM-%03d / %02d", 100+d.cell.number()%900, 1+d.cell.number()%12)},
				{"Production line", fmt.Sprintf("Assembly %02d", 1+d.cell.number()%8)},
				{"Method", "Spectral transmission"},
				{"Conditioning", "23 C / 50% RH"},
			} {
				ui.Column(c).Gap(3).Children(func() {
					dossierCaption(c, field[0])
					ui.Text(c, field[1]).FontSize(16).FontWeight(600)
				})
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Gap(14).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Optical response").FontSize(20).FontWeight(800)
				ui.Spacer(c)
				text := "Reference profile"
				if r.scans > 0 {
					text = fmt.Sprintf("Sample scan %02d", r.scans)
				}
				dossierCaption(c, text).ContentTransition(archiveHUDText)
			})
			d.plot(c, r).Grow(1).MinHeight(160)
			ui.Row(c).FillWidth().Gap(18).Children(func() {
				dossierMetric(c, "Transmission", fmt.Sprintf("%.1f", d.transmission(r, r.wavelength, true)), "%")
				dossierMetric(c, "Haze", fmt.Sprintf("%.1f", 4.2+float64(d.cell.number()%17)*.12), "%")
				dossierMetric(c, "Laminate", "0.42", "mm")
			})
		})
	})
}

func dossierMetric(c *ui.Context, name, value, unit string) {
	ui.Column(c.Key(name)).Grow(1).MinWidth(0).Gap(6).Padding(12, 0).Children(func() {
		dossierCaption(c, name)
		ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, value).Font("Menlo").FontSize(30).LetterSpacing(.5).TextColor(c.Theme().Accent).
				ContentTransition(archiveHUDText)
			ui.Text(c, unit).FontSize(14)
		})
	})
}

func (d *fileDossier) analysis(c *ui.Context, r *laboratoryRecord) {
	ui.Row(c).Fill().Gap(24).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Gap(12).Children(func() {
			ui.Text(c, "Spectral transmission").FontSize(20).FontWeight(800)
			d.plot(c, r).Grow(1).MinHeight(180)
			ui.Row(c).Gap(12).Children(func() {
				ui.Box(c).Size(22, 3).Background(panelGreen)
				ui.Text(c, "Sample").FontSize(13)
				ui.Box(c).Size(22, 3).Background(panelAccent)
				ui.Text(c, "Reference").FontSize(13)
			})
		})
		ui.Column(c).Width(256).Shrink(0).FillHeight().Padding(20).Gap(14).Background(d.theme.Surface).Children(func() {
			dossierCaption(c, "PROBE WAVELENGTH")
			ui.Textf(c, "%.0f nm", r.wavelength).Font("Menlo").FontSize(30).TextColor(d.theme.Accent).
				ContentTransition(archiveHUDText)
			ui.Slider(c.Key("probe-wavelength"), &r.wavelength, 400, 700).FillWidth().Step(5).
				Label("Probe wavelength")
			ui.Checkbox(c.Key("reference-trace"), &r.comparison, "Reference trace").FontSize(15)
			ui.Box(c).FillWidth().Height(1).Background(d.theme.Text.Alpha(.15))
			dossierMetric(c, "Transmission", fmt.Sprintf("%.1f", d.transmission(r, r.wavelength, true)), "%")
			ui.Text(c, "Probe a wavelength or run another simulated sample scan.").
				FontSize(14).LineHeight(1.45).TextColor(d.theme.TextMuted)
		})
	})
}

func (d *fileDossier) notes(c *ui.Context, r *laboratoryRecord) {
	ui.Column(c).Fill().Gap(14).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Laboratory notes").FontSize(20).FontWeight(800)
			ui.Spacer(c)
			status := "Saved in this session"
			if r.draft != r.notes {
				status = "Unsaved changes"
			}
			dossierCaption(c, status).ContentTransition(archiveHUDText)
		})
		ui.TextArea(c.Key("laboratory-notes"), &r.draft).FillWidth().Grow(1).MinHeight(0).
			Padding(20).Radius(0).Background(d.theme.Surface).Border(0, ui.Transparent).
			FontSize(18).LineHeight(1.5).Label("Laboratory notes").
			Placeholder("Record observations, handling instructions, or a review decision...").MaxLength(6000)
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			dossierButton(c, "Save notes", panelGreen, panelPaper, func() { r.notes = r.draft }).Width(150).
				Disabled(r.draft == r.notes)
			dossierButton(c, "Discard edits", ui.Transparent, d.theme.Text, func() { r.draft = r.notes }).Width(150).
				Disabled(r.draft == r.notes)
			ui.Spacer(c)
			ui.Text(c, "Notes stay with this file for the current session.").FontSize(13).TextColor(d.theme.TextMuted)
		})
	})
}

func (d *fileDossier) transmission(r *laboratoryRecord, wavelength float64, measured bool) float64 {
	value := 90.5 + 4.2*math.Exp(-math.Pow((wavelength-555)/105, 2)) - 2.4*math.Exp(-math.Pow((wavelength-430)/28, 2))
	if measured && r.scans > 0 {
		value += .65*math.Sin(wavelength*.035+float64(d.cell.number())*.31+float64(r.scans)*.7) - float64(d.cell.number()%7)*.09
	}
	return value
}

func (d *fileDossier) plot(c *ui.Context, r *laboratoryRecord) ui.Element {
	plot := ui.Box(c.Key("spectral-plot")).FillWidth().Background(d.theme.Surface).
		Label("Transmission spectrum from 400 to 700 nanometers").IgnoreInput(true)
	probe := plot.AnimateTo("probe", float32(r.wavelength), archiveHUDMotion)
	measurement := plot.AnimateTo("measurement", float32(r.scans), archiveHUDMotion)
	plot.Draw(func(p *ui.Painter, bounds ui.Rect) {
		graph := ui.Rect{X: bounds.X + 24, Y: bounds.Y + 22, W: bounds.W - 48, H: bounds.H - 50}
		if graph.W <= 0 || graph.H <= 0 {
			return
		}
		for i := 0; i <= 6; i++ {
			x := graph.X + graph.W*float32(i)/6
			p.Line(x, graph.Y, x, graph.Y+graph.H, 1, d.theme.Text.Alpha(.08))
		}
		for i := 0; i <= 4; i++ {
			y := graph.Y + graph.H*float32(i)/4
			p.Line(graph.X, y, graph.X+graph.W, y, 1, d.theme.Text.Alpha(.08))
		}
		pointAt := func(wavelength float64, sample bool) (float32, float32) {
			value := d.transmission(r, wavelength, false)
			if sample && measurement > 0 {
				value += float64(min(measurement, 1)) * (.65*math.Sin(wavelength*.035+float64(d.cell.number())*.31+float64(measurement)*.7) - float64(d.cell.number()%7)*.09)
			}
			return graph.X + graph.W*float32((wavelength-400)/300), graph.Y + graph.H*float32((100-value)/15)
		}
		for i := 1; i <= 120; i++ {
			a, b := 400+float64(i-1)*2.5, 400+float64(i)*2.5
			if r.comparison {
				x0, y0 := pointAt(a, false)
				x1, y1 := pointAt(b, false)
				p.Line(x0, y0, x1, y1, 2, panelAccent)
			}
			if r.scans > 0 {
				x0, y0 := pointAt(a, true)
				x1, y1 := pointAt(b, true)
				p.Line(x0, y0, x1, y1, 2.5, d.theme.Accent)
			}
		}
		x, y := pointAt(float64(probe), r.scans > 0)
		p.Line(x, graph.Y, x, graph.Y+graph.H, 1, panelRust.Alpha(.6))
		p.Fill(ui.Rect{X: x - 4, Y: y - 4, W: 8, H: 8}, d.theme.Accent, 4)
	})
	plot.Children(func() {
		ui.Row(c).Absolute().Left(24).Right(24).Bottom(8).Children(func() {
			dossierCaption(c, "400 nm")
			ui.Spacer(c)
			dossierCaption(c, "TRANSMISSION / 85-100%")
			ui.Spacer(c)
			dossierCaption(c, "700 nm")
		})
	})
	return plot
}
