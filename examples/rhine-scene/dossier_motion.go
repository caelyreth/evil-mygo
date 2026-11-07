package main

import "time"

const (
	dossierEnterDuration = .62
	dossierExitDuration  = .46
)

// Navigation fades between file covers. Inspection is a separate, reversible
// cover-to-details motion; returning never removes the selected file's cover.
type dossierPresentation struct {
	cell, target archiveCell
	phase        float64
	open         spring
	started      bool
	inspecting   bool
	previous     time.Time
}

func (p *dossierPresentation) advance(now time.Time, hidden, ready, reduced bool, selected archiveCell, inspecting bool) bool {
	before, cell, opened := p.phase, p.cell, p.open.value
	if ready {
		switch {
		case !p.started:
			p.started, p.target = true, selected
			p.previous = now
		case selected != p.target:
			p.target = selected
			p.previous = now
		case inspecting != p.inspecting:
			p.previous = now
		}
		p.inspecting = inspecting
	}
	if hidden || !ready {
		p.previous = time.Time{}
		return false
	}
	dt := 0.0
	if !p.previous.IsZero() {
		dt = min(max(now.Sub(p.previous).Seconds(), 0), .1)
	}
	p.previous = now
	if reduced {
		p.cell, p.phase, p.open = p.target, 1, spring{}
		if p.inspecting {
			p.open.value = 1
		}
	} else if p.cell != p.target {
		p.phase = max(0, p.phase-dt/dossierExitDuration)
		if p.phase == 0 {
			p.cell, p.open = p.target, spring{}
		}
	} else {
		p.phase = min(1, p.phase+dt/dossierEnterDuration)
		target := 0.0
		if p.inspecting {
			target = 1
		}
		p.open.step(target, 7.5, dt)
	}
	return p.phase != before || p.cell != cell || p.open.value != opened
}

func (p *dossierPresentation) moving() bool {
	if !p.started {
		return false
	}
	target := 0.0
	if p.inspecting {
		target = 1
	}
	return p.cell != p.target || p.phase < 1 || p.open.value != target || p.open.velocity != 0
}

func (p *dossierPresentation) interactive() bool {
	return p.started && p.inspecting && p.cell == p.target && p.phase == 1 && p.open.value == 1
}

func dossierCoverage(phase float64) float64 {
	t := min(max(phase, 0), 1)
	return t * t * t * (t*(t*6-15) + 10)
}

type dossierLayers struct {
	cover, tabs, body, footer float32
}

func dossierLayersAt(open float64) dossierLayers {
	return dossierLayers{
		cover:  1 - float32(smooth(open/.52)),
		tabs:   float32(smooth((open - .5) / .28)),
		body:   float32(smooth((open - .58) / .38)),
		footer: float32(smooth((open - .76) / .24)),
	}
}
