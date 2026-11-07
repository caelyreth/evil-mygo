package main

import (
	"testing"
	"time"
)

func TestDossierNavigationFadesCoversWithoutOpeningDetails(t *testing.T) {
	a, b := archiveCell{2, 12}, archiveCell{3, 13}
	p := dossierPresentation{cell: a, target: a}
	now := time.Unix(0, 0)
	p.advance(now, false, true, false, a, false)
	step := func(cell archiveCell, inspecting bool, frames int) {
		for range frames {
			now = now.Add(time.Second / 60)
			p.advance(now, false, true, false, cell, inspecting)
		}
	}
	step(a, false, 90)
	if p.phase != 1 || p.open.value != 0 || p.interactive() || p.moving() {
		t.Fatal("normal navigation did not settle on the dedicated cover")
	}
	now = now.Add(time.Hour)
	p.advance(now, false, true, false, b, false)
	if p.cell != a || p.phase != 1 || p.open.value != 0 || p.interactive() {
		t.Fatal("selection swapped the texture or jumped its exit pose")
	}
	step(b, false, 10)
	if p.cell != a || p.phase <= 0 || p.phase >= 1 {
		t.Fatal("outgoing file did not retain its content while fading")
	}
	step(b, false, 140)
	if p.cell != b || p.phase != 1 || p.open.value != 0 || p.interactive() || p.moving() {
		t.Fatal("incoming cover did not replace the outgoing file without opening details")
	}
}

func TestDossierReturnMorphsBackToCoverAndReversesContinuously(t *testing.T) {
	cell := archiveCell{2, 12}
	p := dossierPresentation{cell: cell, target: cell}
	now := time.Unix(0, 0)
	p.advance(now, false, true, true, cell, true)
	p.advance(now, false, true, false, cell, false)
	now = now.Add(100 * time.Millisecond)
	p.advance(now, false, true, false, cell, false)
	before := p.open
	if before.value <= 0 || before.value >= 1 || p.phase != 1 {
		t.Fatal("Return faded out the file instead of transforming to its cover")
	}
	p.advance(now, true, true, false, cell, false)
	now = now.Add(time.Hour)
	p.advance(now, false, true, false, cell, false)
	if p.open != before || p.phase != 1 {
		t.Fatal("hidden time advanced the file transition")
	}
	p.advance(now, false, true, false, cell, true)
	if p.open != before || p.phase != 1 {
		t.Fatal("reopening reset the pose or velocity instead of reversing the current motion")
	}
	for range 120 {
		now = now.Add(time.Second / 60)
		p.advance(now, false, true, false, cell, true)
		if p.phase != 1 || p.cell != cell {
			t.Fatal("cover-to-details motion blanked or replaced the selected file")
		}
	}
	if !p.interactive() || p.moving() {
		t.Fatal("reopened file did not finish its reveal")
	}
	p.advance(now, false, true, true, cell, false)
	if p.phase != 1 || p.open.value != 0 || p.moving() || p.interactive() {
		t.Fatal("reduced motion did not return directly to a visible cover")
	}
}

func TestDossierCoverAndDetailLayersStayDistinct(t *testing.T) {
	cover, unfolding, details := dossierLayersAt(0), dossierLayersAt(.5), dossierLayersAt(1)
	if cover.cover != 1 || cover.tabs != 0 || cover.body != 0 || cover.footer != 0 {
		t.Fatal("navigation cover exposes detail controls")
	}
	if unfolding.body != 0 || unfolding.footer != 0 {
		t.Fatal("detail content appeared before the specimen register reached its header")
	}
	if details.cover != 0 || details.tabs != 1 || details.body != 1 || details.footer != 1 {
		t.Fatal("details did not finish unfolding or retained the cover")
	}
}

func TestDossierCoverageMeetsPhysicalSurfaceContinuously(t *testing.T) {
	if dossierCoverage(0) != 0 || dossierCoverage(1) != 1 {
		t.Fatal("page blend does not reach both original material and complete UI")
	}
	if dossierCoverage(.001) > .000001 || 1-dossierCoverage(.999) > .000001 {
		t.Fatal("page blend has a visible step at an endpoint")
	}
	previous := 0.0
	for i := 0; i <= 100; i++ {
		value := dossierCoverage(float64(i) / 100)
		if value < previous || value < 0 || value > 1 {
			t.Fatal("page blend reverses or overshoots its material range")
		}
		previous = value
	}
}
