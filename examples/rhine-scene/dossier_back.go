//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Custody is a separate record, UI engine and input surface. Its controls never
// alias the front page's analysis settings, review decision or laboratory notes.
type custodyRecord struct {
	humidity     float64
	seal, hold   bool
	transfers    int
	draft, notes string
}

func (d *fileDossier) custody() *custodyRecord {
	r := d.handling[d.cell]
	if r == nil {
		r = &custodyRecord{humidity: 45}
		d.handling[d.cell] = r
	}
	return r
}

func (d *fileDossier) reversePage(c *ui.Context) {
	r := d.custody()
	layers := dossierLayersAt(d.detail)
	ui.Row(c.Key("custody-letterhead")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).
		Top(dossierInsetTop).Height(32).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, rhineLabLogo).Size(64, 30).TextColor(d.theme.Text)
		ui.Text(c, "RHINE LAB").Font(archiveDisplayFont).FontSize(23).FontWeight(900).LetterSpacing(2)
		ui.Spacer(c)
		dossierCaption(c, "REVERSE / CUSTODY RECORD")
	})
	ui.Box(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(116).Height(3).Background(panelRust)
	ui.Column(c).Absolute().Left(dossierInsetSide).Top(141).Gap(8).Children(func() {
		dossierCaption(c, "MATERIAL HANDLING")
		ui.Text(c, "Handling & provenance").FontSize(42).FontWeight(800).LetterSpacing(.6)
	})
	ui.Row(c).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(216).Height(26).
		AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "RL-%05d", d.cell.number()).Font("Menlo").FontSize(21).LetterSpacing(1)
		ui.Spacer(c)
		dossierCaption(c, d.materialFamily()+" / "+d.batch())
	})
	ui.Row(c.Key("custody-body")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(263).Bottom(196).
		Gap(24).Opacity(layers.body).Translate(0, 18*(1-layers.body)).Children(func() {
		ui.Column(c).Width(292).Shrink(0).FillHeight().Padding(16, 20).Gap(10).Background(d.theme.Surface).Children(func() {
			ui.Text(c, "Storage conditions").FontSize(20).FontWeight(800)
			ui.Text(c, "23 C").Font("Menlo").FontSize(34).TextColor(d.theme.Accent)
			dossierCaption(c, fmt.Sprintf("HUMIDITY LIMIT / %.0f%% RH", r.humidity)).ContentTransition(archiveHUDText)
			ui.Slider(c.Key("custody-humidity"), &r.humidity, 30, 65).FillWidth().Step(1).Label("Custody humidity limit")
			ui.Checkbox(c.Key("custody-seal"), &r.seal, "Seal verified").FontSize(15)
			ui.Checkbox(c.Key("custody-hold"), &r.hold, "Handling hold").FontSize(15)
		})
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Gap(10).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Chain of custody").FontSize(22).FontWeight(800)
				ui.Spacer(c)
				dossierCaption(c, fmt.Sprintf("%02d ENTRIES", 2+r.transfers)).ContentTransition(archiveHUDText)
			})
			for i, entry := range [][2]string{
				{"RECEIVED", "Materials division"}, {"RETAINED", "Controlled sample storage"},
				{"TRANSFER", fmt.Sprintf("%02d recorded this session", r.transfers)},
			} {
				ui.Row(c.Key(entry[0])).FillWidth().Height(43).Gap(16).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(5, 28).Background([3]ui.Color{panelGreen, panelAccent, panelRust}[i])
					dossierCaption(c, entry[0]).Width(92)
					ui.Text(c, entry[1]).FontSize(16).ContentTransition(archiveHUDText)
				})
			}
			ui.Spacer(c)
			dossierButton(c, "Log custody transfer", panelGreen, panelPaper, func() { r.transfers++ }).
				Width(242).Height(38).Disabled(r.hold)
		})
	})
	ui.Column(c.Key("custody-notes")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Top(560).Height(99).
		Gap(7).Opacity(layers.body).Children(func() {
		dossierCaption(c, "HANDLING INSTRUCTIONS")
		ui.TextArea(c.Key("custody-instructions"), &r.draft).FillWidth().Grow(1).MinHeight(0).
			Padding(12, 16).Radius(0).Border(0, ui.Transparent).Background(d.theme.Surface).FontSize(16).
			Label("Custody handling instructions").Placeholder("Packaging, storage, or transfer instructions...").MaxLength(3000)
	})
	ui.Row(c.Key("custody-actions")).Absolute().Left(dossierInsetSide).Right(dossierInsetSide).Bottom(dossierInsetBottom).
		Height(34).Gap(12).AlignItems(ui.Center).Opacity(layers.footer).Children(func() {
		dossierCaption(c, "DEMONSTRATION / SESSION RECORD")
		ui.Spacer(c)
		dossierButton(c, "Discard handling edits", ui.Transparent, d.theme.Text, func() { r.draft = r.notes }).
			Width(208).Height(34).Disabled(r.draft == r.notes)
		dossierButton(c, "Save handling notes", panelAccent, panelInk, func() { r.notes = r.draft }).
			Width(204).Height(34).Disabled(r.draft == r.notes)
	})
}
