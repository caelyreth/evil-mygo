package ui

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/scene"
)

// Pulse configures a repeating cosine opacity animation between Max and Min.
// A zero Period keeps the overlay solid at Max (or 1 when both Min and Max are 0).
type Pulse struct {
	// Start is when the current Delay hold began (for example the time of the
	// last user edit); zero begins Delay when the animation is attached.
	Start time.Time
	// Delay holds Max opacity after Start before oscillating.
	Delay time.Duration
	// Period is the duration of one full cosine cycle (Max -> Min -> Max).
	Period time.Duration
	// Min and Max bound the opacity oscillation in [0, 1]. If both are zero,
	// Max defaults to 1.
	Min, Max float32
	// Interval is the software fallback frame interval while oscillating;
	// zero defaults to 1/30s.
	Interval time.Duration
}

const (
	defaultPulseInterval = time.Second / 30
	contReachPx          = 1.528665
)

func normalizePulse(p Pulse) (minA, maxA float32, period, interval time.Duration) {
	minA = min(max(p.Min, 0), 1)
	maxA = min(max(p.Max, 0), 1)
	if p.Min == 0 && p.Max == 0 {
		maxA = 1
	}
	if maxA < minA {
		maxA = minA
	}
	interval = p.Interval
	if interval <= 0 {
		interval = defaultPulseInterval
	}
	return minA, maxA, p.Period, interval
}

func pulseOpacity(p Pulse, now time.Time) float32 {
	minA, maxA, period, _ := normalizePulse(p)
	if period <= 0 {
		return maxA
	}
	if !p.Start.IsZero() {
		since := now.Sub(p.Start)
		if since < p.Delay {
			return maxA
		}
		phase := (since - p.Delay).Seconds() / period.Seconds() * 2 * math.Pi
		t := float32(0.5 + 0.5*math.Cos(phase))
		return minA + (maxA-minA)*t
	}
	return maxA
}

func pulseNextWake(p Pulse, now time.Time) (time.Duration, bool) {
	_, _, period, interval := normalizePulse(p)
	if period <= 0 {
		return 0, false
	}
	if !p.Start.IsZero() {
		since := now.Sub(p.Start)
		if since < p.Delay {
			return p.Delay - since, true
		}
		return interval, true
	}
	if p.Delay > 0 {
		return p.Delay, true
	}
	return interval, true
}

type partKey struct {
	e     *node
	after bool
}

type pendingOverlay struct {
	id         uint64
	ownerID    uint64
	inDraw     bool
	partKey    partKey
	partSlot   int
	part       paintPart
	seq        uint32
	opIndex    int
	pxFrame    scene.Rect
	pxClip     scene.Rect
	frame      Rect
	clip       Rect
	radius     float32
	scale      float32
	color      Color
	minA       float32
	maxA       float32
	period     time.Duration
	interval   time.Duration
	pulse      Pulse
	now        time.Time
	occluded   bool
	clippedOut bool
	spliced    bool
}

// OverlayFill draws a solid rounded rectangle of color c and corner radius at
// r, pulsing opacity according to pulse. When the surface supports compositor
// overlays and the overlay sits unobstructed inside rectangular clips, the OS
// compositor renders and animates it without waking the frame loop; otherwise
// OverlayFill falls back cleanly to in-scene drawing and timed repaints.
func (p *Painter) OverlayFill(r Rect, c Color, radius float32, pulse Pulse) {
	if r.W <= 0 || r.H <= 0 || p.opacity <= 0 || c.A == 0 {
		return
	}
	now := p.Now()
	if vis, ok := p.canOverlay(r, c); ok {
		pxFrame := p.snap(r)
		pxClip := pxFrame.Intersect(p.snap(vis))
		if pxFrame.Empty() || pxClip.Empty() {
			return
		}
		rt := p.rt
		minA, maxA, period, interval := normalizePulse(pulse)
		var ownerID uint64
		if p.owner != nil {
			ownerID = p.owner.id
		}
		after := p.drawing && rt.activePart.after
		sub := 0
		if p.drawing {
			for i := rt.curDrawOvStart; i < len(rt.overlays); i++ {
				if rt.overlays[i].ownerID == ownerID {
					sub++
				}
			}
		} else {
			for i := range rt.overlays {
				if !rt.overlays[i].inDraw && rt.overlays[i].ownerID == ownerID {
					sub++
				}
			}
		}
		var subTag uint64 = uint64(sub) << 1
		if after {
			subTag |= 1
		}
		id := mix(ownerID, subTag+1)
		if ownerID == 0 {
			id = uint64(len(rt.overlays) + 1)
		}
		rt.paintSeq++
		rt.overlays = append(rt.overlays, pendingOverlay{
			id:       id,
			ownerID:  ownerID,
			inDraw:   p.drawing,
			partKey:  partKey{e: rt.activePart.e, after: rt.activePart.after},
			partSlot: rt.curPartSlot,
			seq:      rt.paintSeq,
			opIndex:  len(p.s.Ops),
			pxFrame:  pxFrame,
			pxClip:   pxClip,
			frame:    r,
			clip:     vis,
			radius:   max(radius, 0),
			scale:    p.scale,
			color:    c,
			minA:     minA,
			maxA:     maxA,
			period:   period,
			interval: interval,
			pulse:    pulse,
			now:      now,
		})
		return
	}
	alpha := pulseOpacity(pulse, now)
	if alpha > 0 {
		p.Fill(r, c.Alpha(alpha), radius)
	}
	if p.rec == nil && p.visible(r, 0) {
		if wait, ok := pulseNextWake(pulse, now); ok {
			p.After(wait)
		}
	}
}

func (p *Painter) canOverlay(r Rect, c Color) (Rect, bool) {
	if p.rt == nil || p.s != &p.rt.scene || p.rec != nil || p.copying || p.rt.insp.open {
		return Rect{}, false
	}
	if p.transform.Set || p.opacity < 0.999 || c.wide.ok() {
		return Rect{}, false
	}
	if !p.rt.host.compositorOverlays() {
		return Rect{}, false
	}
	vis := intersect(r, p.clip)
	if vis.W <= 0 || vis.H <= 0 {
		return Rect{}, false
	}
	return vis, true
}

type overlayClip struct {
	rect   scene.Rect
	area   scene.Rect
	reach  [4]float32
	masked bool
}

func pushOverlayClip(stack []overlayClip, op *scene.Op) []overlayClip {
	o := scene.MovedClipOp(op)
	c := overlayClip{
		rect:   o.Rect,
		area:   o.Transform.Bounds(o.Rect),
		masked: o.Transform.Set || o.Paint != scene.PaintSolid || o.Image != nil,
	}
	if len(stack) > 0 {
		c.area = c.area.Intersect(stack[len(stack)-1].area)
	}
	if !c.masked {
		ext := float32(1)
		if o.Continuous {
			ext = contReachPx
		}
		for i, r := range scene.FitRadii(o.Rect, o.Radii) {
			if r > 0 {
				c.reach[i] = r*ext + 1
			}
		}
	}
	return append(stack, c)
}

func (c *overlayClip) hitsCorner(r scene.Rect) bool {
	if c.masked {
		return true
	}
	for i, k := range c.reach {
		if k <= 0 {
			continue
		}
		var corner scene.Rect
		switch i {
		case 0:
			corner = scene.Rect{X: c.rect.X, Y: c.rect.Y, W: k, H: k}
		case 1:
			corner = scene.Rect{X: c.rect.X + c.rect.W - k, Y: c.rect.Y, W: k, H: k}
		case 2:
			corner = scene.Rect{X: c.rect.X + c.rect.W - k, Y: c.rect.Y + c.rect.H - k, W: k, H: k}
		case 3:
			corner = scene.Rect{X: c.rect.X, Y: c.rect.Y + c.rect.H - k, W: k, H: k}
		}
		if !corner.Intersect(r).Empty() {
			return true
		}
	}
	return false
}

func outsetRect(r scene.Rect, m float32) scene.Rect {
	if m == 0 || r.Empty() {
		return r
	}
	return scene.Rect{X: r.X - m, Y: r.Y - m, W: r.W + 2*m, H: r.H + 2*m}
}

func opOverlapsRect(s *scene.Scene, op *scene.Op, clipArea, target scene.Rect) bool {
	if target.Empty() || clipArea.Intersect(target).Empty() {
		return false
	}
	switch op.Kind {
	case scene.OpFill:
		if op.Paint == scene.PaintSolid && op.Color.A == 0 && (!scene.HasBorder(op.Border) || op.BorderColor.A == 0) && op.Wide == 0 {
			return false
		}
		b := op.Transform.Bounds(op.Rect)
		if op.Transform.Set || op.Radii != [4]float32{} || scene.HasBorder(op.Border) {
			b = outsetRect(b, 1)
		}
		return !b.Intersect(clipArea).Intersect(target).Empty()
	case scene.OpHole, scene.OpImage, scene.OpEffect:
		b := op.Transform.Bounds(op.Rect)
		if op.Transform.Set {
			b = outsetRect(b, 1)
		}
		return !b.Intersect(clipArea).Intersect(target).Empty()
	case scene.OpShadow:
		var box scene.Rect
		if op.Inset {
			box = op.Cast
		} else {
			e := 1.5*op.Blur + 1
			box = scene.Rect{X: op.Rect.X - e, Y: op.Rect.Y - e, W: op.Rect.W + 2*e, H: op.Rect.H + 2*e}
		}
		if box.Empty() {
			return false
		}
		b := op.Transform.Bounds(box)
		return !b.Intersect(clipArea).Intersect(target).Empty()
	case scene.OpGlyphs:
		if op.Start >= op.End || int(op.End) > len(s.Glyphs) {
			return false
		}
		for _, g := range s.Glyphs[op.Start:op.End] {
			w, h := max(g.W, float32(g.UW)), max(g.H, float32(g.VH))
			gr := scene.Rect{X: g.X, Y: g.Y, W: w, H: h}
			if g.Transform.Set {
				gr = g.Transform.Bounds(gr)
			}
			if op.Transform.Set {
				gr = op.Transform.Bounds(gr)
			}
			if !outsetRect(gr, 1).Intersect(clipArea).Intersect(target).Empty() {
				return true
			}
		}
		return false
	}
	return false
}

func (rt *engine) resolveOverlays(s *scene.Scene, partial bool) {
	rt.overlayPlaces = rt.overlayPlaces[:0]
	if len(rt.overlays) == 0 {
		return
	}
	var stackBuf [8]overlayClip
	stack := append(stackBuf[:0], overlayClip{area: scene.Rect{W: float32(s.Width), H: float32(s.Height)}})
	var activeBuf [8]int
	active := activeBuf[:0]
	nextOv := 0
	checkAt := func(opIdx int) {
		topArea := stack[len(stack)-1].area
		for nextOv < len(rt.overlays) && rt.overlays[nextOv].opIndex == opIdx {
			ov := &rt.overlays[nextOv]
			ov.occluded, ov.clippedOut = false, false
			if ov.pxClip.Intersect(topArea).Empty() {
				ov.clippedOut = true
			} else {
				for k := range stack {
					if stack[k].hitsCorner(ov.pxClip) {
						ov.occluded = true
						break
					}
				}
				if !ov.occluded {
					active = append(active, nextOv)
				}
			}
			nextOv++
		}
	}

	nOps := len(s.Ops)
	for i := range nOps {
		checkAt(i)
		op := &s.Ops[i]
		switch op.Kind {
		case scene.OpPushClip:
			stack = pushOverlayClip(stack, op)
		case scene.OpPopClip:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		default:
			topArea := stack[len(stack)-1].area
			kept := active[:0]
			for _, idx := range active {
				ov := &rt.overlays[idx]
				if ov.spliced && ov.opIndex == i {
					kept = append(kept, idx)
					continue
				}
				if opOverlapsRect(s, op, topArea, ov.pxClip) {
					ov.occluded = true
				} else {
					kept = append(kept, idx)
				}
			}
			active = kept
		}
	}
	checkAt(nOps)

	// Check whether any overlay needs an op inserted (occluded && !spliced) or
	// removed (!occluded && spliced).
	needEdit := false
	for i := range rt.overlays {
		ov := &rt.overlays[i]
		wantSpliced := ov.occluded && !ov.clippedOut
		if wantSpliced != ov.spliced {
			needEdit = true
			break
		}
	}
	if needEdit {
		rt.applyOverlaySplices(s, partial)
	}

	for i := range rt.overlays {
		ov := &rt.overlays[i]
		if ov.clippedOut {
			continue
		}
		if ov.occluded {
			if wait, ok := pulseNextWake(ov.pulse, ov.now); ok {
				at := rt.c.now.Add(wait)
				if rt.repaintAt.IsZero() || at.Before(rt.repaintAt) {
					rt.repaintAt = at
				}
				if ov.inDraw {
					rt.ensureOverlayPart(ov)
				} else {
					rt.untracked = true
				}
			}
			continue
		}
		frame := rt.windowRect(ov.frame)
		clip := rt.windowRect(ov.clip)
		if clip.W <= 0 || clip.H <= 0 {
			continue
		}
		radius := float64(ov.radius * rt.zoomFactor())
		rt.overlayPlaces = append(rt.overlayPlaces, platform.OverlayPlacement{
			ID:           ov.id,
			Frame:        rectF(frame),
			Clip:         rectF(clip),
			CornerRadius: radius,
			Color:        platform.Color{R: ov.color.R, G: ov.color.G, B: ov.color.B, A: ov.color.A},
			Opacity:      ov.maxA,
			PulseStart:   ov.pulse.Start,
			PulseDelay:   ov.pulse.Delay,
			PulsePeriod:  ov.period,
			PulseMin:     ov.minA,
			PulseMax:     ov.maxA,
		})
	}
}

func (rt *engine) applyOverlaySplices(s *scene.Scene, partial bool) {
	oldOps := s.Ops
	nOld := len(oldOps)
	ops := make([]scene.Op, 0, nOld+len(rt.overlays))
	editOutsidePartial := false
	partialDelta := 0

	noteEdit := func(oldPos, delta int) {
		if !partial || s.Partial.BaseID == 0 {
			return
		}
		if oldPos < s.Partial.OpStart || oldPos > s.Partial.NewOpEnd {
			editOutsidePartial = true
		} else {
			partialDelta += delta
		}
	}

	// Build spliced op slice while updating each overlay's opIndex and spliced state.
	// Note rt.overlays is ordered by non-decreasing opIndex.
	pos := 0
	ovIdx := 0
	// Track old->new mapping boundaries for rt.drawn shifts:
	// First record deltas per overlay operation so we can update rt.drawn and
	// ov.part bounds accurately after building ops.
	type spliceDelta struct {
		oldPos int
		delta  int
		ovPtr  *pendingOverlay
	}
	var deltas []spliceDelta

	for pos <= nOld {
		// Emit any overlay actions anchored at pos.
		for ovIdx < len(rt.overlays) && rt.overlays[ovIdx].opIndex == pos {
			ov := &rt.overlays[ovIdx]
			ovIdx++
			wantSpliced := ov.occluded && !ov.clippedOut
			switch {
			case wantSpliced && !ov.spliced:
				ov.opIndex = len(ops)
				ov.spliced = true
				alpha := pulseOpacity(ov.pulse, ov.now)
				col := ov.color.Alpha(alpha)
				ops = append(ops, scene.Op{
					Kind:       scene.OpFill,
					Rect:       ov.pxFrame,
					Radii:      scene.Uniform(ov.radius * ov.scale),
					Continuous: continuousCorners,
					Color:      col.scene(),
					Opacity:    1,
				})
				noteEdit(pos, 1)
				deltas = append(deltas, spliceDelta{oldPos: pos, delta: 1, ovPtr: ov})
			case !wantSpliced && ov.spliced:
				// Delete oldOps[pos].
				ov.spliced = false
				ov.opIndex = len(ops)
				if pos < nOld {
					pos++
				}
				noteEdit(pos-1, -1)
				deltas = append(deltas, spliceDelta{oldPos: pos - 1, delta: -1, ovPtr: ov})
			default:
				ov.opIndex = len(ops)
			}
		}
		if pos >= nOld {
			break
		}
		ops = append(ops, oldOps[pos])
		pos++
	}
	clear(oldOps)
	s.Ops = ops

	if partial && s.Partial.BaseID != 0 {
		if editOutsidePartial {
			s.Partial = scene.PartEdit{}
		} else {
			s.Partial.NewOpEnd += partialDelta
		}
	}

	// Shift rt.drawn and kept part bounds relative to their pre-splice spans.
	shiftPart := func(pt *paintPart) {
		origStart, origEnd := pt.start, pt.end
		var dStart, dEnd int
		for _, sd := range deltas {
			ov := sd.ovPtr
			switch {
			case sd.oldPos < origStart || (sd.oldPos == origStart && (!ov.inDraw || ov.partKey != (partKey{e: pt.e, after: pt.after})) && ov.seq < pt.seq):
				dStart += sd.delta
				dEnd += sd.delta
			case ov.inDraw && ov.partKey == (partKey{e: pt.e, after: pt.after}):
				dEnd += sd.delta
			case sd.oldPos > origStart && sd.oldPos < origEnd:
				dEnd += sd.delta
			}
		}
		pt.start = origStart + dStart
		pt.end = origEnd + dEnd
	}
	for i := range rt.drawn {
		shiftPart(&rt.drawn[i])
	}
	for i := range rt.overlays {
		other := &rt.overlays[i]
		if other.inDraw && other.part.e != nil {
			shiftPart(&other.part)
		}
	}
}

func (rt *engine) ensureOverlayPart(ov *pendingOverlay) {
	if ov.part.e == nil {
		return
	}
	for i := range rt.drawn {
		if rt.drawn[i].e == ov.part.e && rt.drawn[i].after == ov.part.after {
			return
		}
	}
	rt.partTransient += ov.part.transientDelta
	rt.paths.spill.inParts += ov.part.spillDelta
	pos := 0
	for pos < len(rt.drawn) && (rt.drawn[pos].start < ov.part.start ||
		(rt.drawn[pos].start == ov.part.start && rt.drawn[pos].seq < ov.part.seq)) {
		pos++
	}
	rt.drawn = slices.Insert(rt.drawn, pos, ov.part)
}

func (rt *engine) placeOverlays() {
	if len(rt.overlayPlaces) == 0 && !rt.overlaysShown {
		return
	}
	rt.overlaysShown = len(rt.overlayPlaces) > 0
	rt.host.placeOverlays(rt.overlayPlaces)
}
