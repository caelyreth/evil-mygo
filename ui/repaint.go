package ui

import "github.com/egoist/mygo/internal/scene"

// Frames that only paint again. A drawing that moves with the time
// (Painter.AnimationFrame, AnimationFrameRate, After) while nothing else
// changes has the next frame paint the elements of this one again, without
// building the view; yet painting them all again, every background, text
// run and Draw callback of the window, costs as much as what the window
// shows. So the frame painting the window keeps, for each Draw or DrawOver
// callback that asked to be painted again, the operations of the scene it
// painted and the painter as it was: a frame that only paints again runs
// those callbacks alone, as they were, and puts what they paint in the
// place of what they painted, in the last frame's scene. Anything else
// asking for such a frame (a pose moving, a drawing in a canvas or a
// layer, content changing) has the frame paint the window whole, as do the
// inspector and a drag.

// paintPart is a Draw (or, after, DrawOver) callback of element e, called
// with box, that painted the operations of the window's scene from start
// to end, with the painter as it was then, its opaque backgrounds and holes
// apart.
type paintPart struct {
	e          *node
	after      bool
	box        Rect
	start, end int
	painter    Painter
	under      []opaqueBox
	holes      []Rect
	// from and to are where it painted the frame being painted, at the
	// end of the scene's operations.
	from, to       int
	seq            uint32
	transientDelta uint64
	spillDelta     uint64
}

// draw calls e's Draw callback (DrawOver's when after) with box, noting
// what it painted when it asks to be painted again (paintPart).
func (p *Painter) draw(e *node, box Rect, after bool) {
	fn := e.paintFn
	if after {
		fn = e.paintAfterFn
	}
	if p.drawing || p.s != &p.rt.scene {
		// Into a layer or a canvas, painted whole.
		fn(p, box)
		return
	}
	rt := p.rt
	start, state, uses := len(p.s.Ops), *p, rt.text.TransientUses()
	// Layers of paths drawn on the CPU end where a part does, which is
	// painted again alone (pathSpill).
	sp := &rt.paths.spill
	sp.run++
	opened := sp.opened
	rt.paintSeq++
	rt.activePart = paintPart{
		e: e, after: after, box: box, start: start, end: -1,
		painter: state, seq: rt.paintSeq,
	}
	rt.curDrawOvStart = len(rt.overlays)
	p.drawing, p.asked = true, false
	fn(p, box)
	p.drawing = false
	sp.run++
	hadOv := rt.curDrawOvStart < len(rt.overlays)
	if p.asked || hadOv {
		rt.activePart.end = len(p.s.Ops)
		rt.activePart.under = append([]opaqueBox(nil), state.under...)
		rt.activePart.holes = append([]Rect(nil), state.holes...)
		rt.activePart.transientDelta = rt.text.TransientUses() - uses
		rt.activePart.spillDelta = sp.opened - opened
		if p.asked {
			rt.partTransient += rt.activePart.transientDelta
			sp.inParts += rt.activePart.spillDelta
			rt.drawn = append(rt.drawn, rt.activePart)
		} else {
			for i := rt.curDrawOvStart; i < len(rt.overlays); i++ {
				rt.overlays[i].part = rt.activePart
			}
		}
	}
	rt.activePart = paintPart{}
}

// asks notes that the drawing painting asks to be painted again: by
// itself, as its part, from a Draw or DrawOver callback of the window's
// painting, else with the whole window.
func (p *Painter) asks() {
	if p.drawing {
		p.asked = true
	} else {
		p.rt.untracked = true
	}
}

// partMark records the scene slice lengths after a whole paint so
// repaintParts can truncate away prior partial-frame tails instead of
// accumulating scene data across frames.
type partMark struct {
	glyphs, stops, effects, wide, words int
}

// repaintParts paints the parts of the last frame painted again (see
// paintPart), and reports whether it could: false when the window must be
// painted whole, as when the glyph atlases, which every window shares,
// moved or forgot the glyphs the rest of the scene draws, as another
// window making room in them does, or when the rest of the scene draws
// masks drawn for its frame alone, in room of the atlas that the frames
// since drew over: painted whole again, they last.
func (rt *engine) repaintParts() bool {
	s := &rt.scene
	if rt.untracked || len(rt.drawn) == 0 || rt.insp.open || rt.drag != nil || rt.noParts ||
		atlasGenerations(s) != rt.atlases || rt.transient {
		return false
	}
	rt.painting = true
	defer func() { rt.painting = false }()
	rt.partPaints++
	rt.paths.spill.beginPaint()
	n := len(s.Ops)
	baseID := s.ID
	firstStart := rt.drawn[0].start
	lastOldEnd := rt.drawn[len(rt.drawn)-1].end
	validSpan := baseID != 0 && firstStart >= 0 && firstStart <= lastOldEnd && lastOldEnd <= n
	prevEnd := 0
	for i := range rt.drawn {
		if rt.drawn[i].start < prevEnd || rt.drawn[i].end < rt.drawn[i].start || rt.drawn[i].end > lastOldEnd {
			validSpan = false
			break
		}
		prevEnd = rt.drawn[i].end
	}
	// The canvases the last frame drew are drawn: those the parts draw
	// anew are this frame's.
	clear(s.Canvases)
	s.Canvases = s.Canvases[:0]
	// Reclaim tail data appended by previous repaintParts frames; ops outside
	// rt.drawn only reference indices below rt.partMark, and every part in
	// rt.drawn is repainted below.
	s.Glyphs = s.Glyphs[:rt.partMark.glyphs]
	s.Stops = s.Stops[:rt.partMark.stops]
	clear(s.Effects[rt.partMark.effects:])
	s.Effects = s.Effects[:rt.partMark.effects]
	s.Wide = s.Wide[:rt.partMark.wide]
	s.Words = s.Words[:rt.partMark.words]

	// Retain overlays belonging to untouched parts or non-Draw elements while
	// removing those owned by parts about to repaint.
	var keptOverlays []pendingOverlay
	if len(rt.overlays) > 0 {
		for _, ov := range rt.overlays {
			repainted := false
			if ov.inDraw {
				for i := range rt.drawn {
					if rt.drawn[i].e == ov.partKey.e && rt.drawn[i].after == ov.partKey.after {
						repainted = true
						break
					}
				}
			}
			if !repainted {
				keptOverlays = append(keptOverlays, ov)
			}
		}
		rt.overlays = rt.overlays[:0]
	}

	for i := range rt.drawn {
		pt := &rt.drawn[i]
		rt.painter = pt.painter
		p := &rt.painter
		p.under = append(rt.under[:0], pt.under...)
		p.holes = append(rt.holes[:0], pt.holes...)
		fn := pt.e.paintFn
		if pt.after {
			fn = pt.e.paintAfterFn
		}
		// A part that no longer asks to be painted again stays, as what
		// it painted last stays in the scene.
		pt.from = len(s.Ops)
		rt.curPartSlot = i
		rt.activePart = *pt
		rt.curDrawOvStart = len(rt.overlays)
		p.drawing, p.asked = true, false
		rt.paths.spill.run++
		fn(p, pt.box)
		rt.paths.spill.run++
		p.drawing = false
		pt.to = len(s.Ops)
		for j := rt.curDrawOvStart; j < len(rt.overlays); j++ {
			rt.overlays[j].opIndex -= pt.from
			rt.overlays[j].part = *pt
		}
		rt.activePart = paintPart{}
		rt.under, rt.holes = p.under[:0], p.holes[:0]
	}

	var mergedOverlays []pendingOverlay
	keptIdx, newIdx := 0, 0
	ops, last := rt.partOps[:0], 0
	for i := range rt.drawn {
		pt := &rt.drawn[i]
		gapNew := len(ops)
		ops = append(ops, s.Ops[last:pt.start]...)
		for keptIdx < len(keptOverlays) {
			k := &keptOverlays[keptIdx]
			if !(k.opIndex < pt.start || (k.opIndex == pt.start && k.seq < pt.seq)) {
				break
			}
			k.opIndex = gapNew + (k.opIndex - last)
			mergedOverlays = append(mergedOverlays, *k)
			keptIdx++
		}
		start := len(ops)
		ops = append(ops, s.Ops[pt.from:pt.to]...)
		newEnd := len(ops)
		for newIdx < len(rt.overlays) && rt.overlays[newIdx].partSlot == i {
			ov := rt.overlays[newIdx]
			ov.opIndex = start + ov.opIndex
			ov.part.start = start
			ov.part.end = newEnd
			mergedOverlays = append(mergedOverlays, ov)
			newIdx++
		}
		last, pt.start, pt.end = pt.end, start, newEnd
	}
	sufNew := len(ops)
	ops = append(ops, s.Ops[last:n]...)
	for keptIdx < len(keptOverlays) {
		k := keptOverlays[keptIdx]
		k.opIndex = sufNew + (k.opIndex - last)
		mergedOverlays = append(mergedOverlays, k)
		keptIdx++
	}
	rt.overlays = append(rt.overlays[:0], mergedOverlays...)
	clear(s.Ops)
	rt.partOps, s.Ops = s.Ops[:0], ops
	lastNewEnd := rt.drawn[len(rt.drawn)-1].end
	s.ID = scene.NextSceneID()
	if validSpan && lastNewEnd >= firstStart && lastNewEnd <= len(s.Ops) {
		s.Partial = scene.PartEdit{
			BaseID:     baseID,
			OpStart:    firstStart,
			OldOpEnd:   lastOldEnd,
			NewOpEnd:   lastNewEnd,
			GlyphMark:  rt.partMark.glyphs,
			StopMark:   rt.partMark.stops,
			EffectMark: rt.partMark.effects,
			WideMark:   rt.partMark.wide,
			WordMark:   rt.partMark.words,
		}
	} else {
		s.Partial = scene.PartEdit{}
	}
	rt.finishSpills()
	rt.resolveOverlays(s, true)
	return true
}
