//go:build darwin && !ios

package darwin

import (
	"errors"
	"math"
	"slices"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/platform"
)

// An embedded view in a window of native UI (platform.ViewSurface.EmbedView)
// is a child view hosted inside a clip view and pane under the surface,
// scaled and clipped to its content placement.

// nsViewMinYMargin is NSViewMinYMargin: the margin below a view grows,
// which keeps its distance to the top of an unflipped superview.
const nsViewMinYMargin = 8

// EmbedView puts a view of a native module under the surface, which places
// and clips it alongside the UI.
func (s *surface) EmbedView(view uintptr) (platform.EmbeddedView, error) {
	host := s.w
	if host.closed {
		return nil, errors.New("darwin: the window is closed")
	}
	v := &window{b: host.b, h: host.h, opts: &platform.WindowOptions{}, win: host.win, host: host, embeddedView: id(view)}
	withPool(func() {
		retain(v.embeddedView)
		send(v.embeddedView, "removeFromSuperview")
		send(v.embeddedView, "setAutoresizingMask:", 0)
		v.clip = msgInitRect(send(class("MyGoClipView"), "alloc"), sel("initWithFrame:"), NSRect{})
		send(v.clip, "setWantsLayer:", 1)
		send(send(v.clip, "layer"), "setMasksToBounds:", 1)
		send(v.clip, "setAutoresizingMask:", nsViewMinYMargin)
		send(v.clip, "setHidden:", 1)
		v.pane = msgInitRect(send(class("MyGoClipView"), "alloc"), sel("initWithFrame:"), NSRect{})
		send(v.pane, "setWantsLayer:", 1)
		send(send(v.pane, "layer"), "setMasksToBounds:", 1)
		send(v.pane, "addSubview:", uintptr(v.embeddedView))
		send(v.clip, "addSubview:", uintptr(v.pane))
		send(host.view, "addSubview:positioned:relativeTo:", uintptr(v.clip), nsWindowBelow, uintptr(s.view))
	})
	host.embeddedViews = append(host.embeddedViews, v)
	return v, nil
}

// nsWindowBelow is NSWindowBelow, for addSubview:positioned:relativeTo:.
const nsWindowBelow = ^uintptr(0)

// PlaceViews shows the window's embedded views where the content shows
// them, in its paint order under the surface, and hides the others.
func (s *surface) PlaceViews(views []platform.ViewPlacement) {
	host := s.w
	if host.closed {
		return
	}
	s.placed = append(s.placed[:0], views...)
	height := msgRect(host.view, sel("bounds")).Size.Height
	shown := make([]*window, 0, len(views))
	withPool(func() {
		for _, p := range views {
			v, ok := p.View.(*window)
			if !ok || v.host != host || v.closed {
				continue
			}
			if send(v.embeddedView, "superview") != v.pane {
				// A view of a native module that another window showed
				// since: back to this one.
				send(v.pane, "addSubview:", uintptr(v.embeddedView))
			}
			c, f := p.Clip, p.Frame
			pw, ph := p.Size()
			msgSetRect(v.clip, sel("setFrame:"), NSRect{Origin: NSPoint{c.X, height - c.Y - c.H}, Size: NSSize{c.W, c.H}})
			msgSetRect(v.pane, sel("setFrame:"), NSRect{Origin: NSPoint{f.X - c.X, f.Y - c.Y}, Size: NSSize{f.W, f.H}})
			msgSetSize(v.pane, sel("setBoundsSize:"), NSSize{pw, ph})
			msgSetRect(v.embeddedView, sel("setFrame:"), NSRect{Size: NSSize{pw, ph}})
			v.roundPane(p.Radii, pw, ph)
			msgSetFloat(v.clip, sel("setAlphaValue:"), p.Opacity)
			if sendBool(v.clip, "isHidden") {
				send(v.clip, "setHidden:", 0)
			}
			shown = append(shown, v)
		}
		for _, v := range host.embeddedViews {
			if !slices.Contains(shown, v) && !sendBool(v.clip, "isHidden") {
				if v.hasKeyboard() {
					// A hidden view keeps no keyboard: it goes back to the
					// content.
					send(host.win, "makeFirstResponder:", uintptr(s.view))
				}
				send(v.clip, "setHidden:", 1)
			}
		}
		s.stackViews(shown)
		if len(s.glassShown) > 0 {
			s.stackGlass(s.glassShown, true)
		}
	})
}

var (
	paneOnce        sync.Once
	msgPointToLayer func(obj id, sel objc.SEL, p NSPoint) NSPoint
	cgPathCreate    func() uintptr
	cgPathMoveTo    func(path, transform uintptr, x, y float64)
	cgPathArcTo     func(path, transform uintptr, x1, y1, x2, y2, radius float64)
	cgPathClose     func(path uintptr)
	cgPathRelease   func(path uintptr)
)

// roundPane cuts the pane of the web view w, its bounds width by height,
// to corners of radii, in its bounds' units, from the top-left clockwise,
// as continuous as the content's when they are the same.
func (w *window) roundPane(radii [4]float64, width, height float64) {
	paneOnce.Do(func() {
		purego.RegisterFunc(&msgPointToLayer, msgSendAddr)
		purego.RegisterLibFunc(&cgPathCreate, libCG, "CGPathCreateMutable")
		purego.RegisterLibFunc(&cgPathMoveTo, libCG, "CGPathMoveToPoint")
		purego.RegisterLibFunc(&cgPathArcTo, libCG, "CGPathAddArcToPoint")
		purego.RegisterLibFunc(&cgPathClose, libCG, "CGPathCloseSubpath")
		purego.RegisterLibFunc(&cgPathRelease, libCG, "CGPathRelease")
	})
	layer := send(w.pane, "layer")
	if radii == [4]float64{} {
		msgSetFloat(layer, sel("setCornerRadius:"), 0)
		w.unmask(layer)
		return
	}
	// The pane's corners in its layer, which AppKit scales and flips as
	// it does: by the layer's own transform, or its sublayers'.
	toLayer := func(x, y float64) NSPoint { return msgPointToLayer(w.pane, sel("convertPointToLayer:"), NSPoint{x, y}) }
	corners := [4]NSPoint{toLayer(0, 0), toLayer(width, 0), toLayer(width, height), toLayer(0, height)}
	k := math.Abs(corners[1].X-corners[0].X) / width // the layer's units per the bounds'
	same, r := true, 0.0
	for _, v := range radii {
		if v > 0 && r > 0 && v != r {
			same = false
		}
		r = max(r, v)
	}
	if same {
		// kCALayerMinXMinYCorner and the others, for the corners whose
		// radius is not 0, by where they are in the layer.
		var mask uint
		for i, v := range radii {
			if v <= 0 {
				continue
			}
			p, o := corners[i], corners[(i+2)%4]
			bit := uint(1)
			if p.X > o.X {
				bit <<= 1
			}
			if p.Y > o.Y {
				bit <<= 2
			}
			mask |= bit
		}
		w.unmask(layer)
		msgSetFloat(layer, sel("setCornerRadius:"), r*k)
		send(layer, "setCornerCurve:", uintptr(nsString("continuous")))
		send(layer, "setMaskedCorners:", uintptr(mask))
		return
	}
	// Corners of different radii, circular: a mask of their path, at the
	// layer's bounds, which may not start at 0, 0.
	msgSetFloat(layer, sel("setCornerRadius:"), 0)
	b := msgRect(layer, sel("bounds"))
	for i := range corners {
		corners[i].X -= b.Origin.X
		corners[i].Y -= b.Origin.Y
	}
	path := cgPathCreate()
	mid := func(a, b NSPoint) NSPoint { return NSPoint{(a.X + b.X) / 2, (a.Y + b.Y) / 2} }
	start := mid(corners[3], corners[0])
	cgPathMoveTo(path, 0, start.X, start.Y)
	for i := range 4 {
		p, next := corners[i], mid(corners[i], corners[(i+1)%4])
		cgPathArcTo(path, 0, p.X, p.Y, next.X, next.Y, radii[i]*k)
	}
	cgPathClose(path)
	send(class("CATransaction"), "begin")
	send(class("CATransaction"), "setDisableActions:", 1)
	if w.mask == 0 {
		w.mask = send(send(class("CAShapeLayer"), "alloc"), "init")
		send(layer, "setMask:", uintptr(w.mask))
	}
	msgSetRect(w.mask, sel("setFrame:"), b)
	send(w.mask, "setPath:", path)
	send(class("CATransaction"), "commit")
	cgPathRelease(path)
}

// unmask takes away the mask of the pane's layer.
func (w *window) unmask(layer id) {
	if w.mask != 0 {
		send(layer, "setMask:", 0)
		release(w.mask)
		w.mask = 0
	}
}

// stackViews puts the clip views of the embedded views shown in the order
// the content painted them, under the surface, unless they are.
func (s *surface) stackViews(shown []*window) {
	if len(shown) < 2 {
		return
	}
	subviews := send(s.w.view, "subviews")
	n := sendInt(subviews, "count")
	at := 0
	for i := range n {
		if at < len(shown) && send(subviews, "objectAtIndex:", uintptr(i)) == shown[at].clip {
			at++
		}
	}
	if at == len(shown) {
		return
	}
	// Moving a view out of its superview takes the keyboard from it: the
	// first responder goes back to it.
	first := send(s.w.win, "firstResponder")
	for _, v := range shown {
		retain(v.clip)
		send(v.clip, "removeFromSuperview")
		send(s.w.view, "addSubview:positioned:relativeTo:", uintptr(v.clip), nsWindowBelow, uintptr(s.view))
		release(v.clip)
	}
	if first != 0 && first != send(s.w.win, "firstResponder") {
		send(s.w.win, "makeFirstResponder:", uintptr(first))
	}
}

// viewAt returns the embedded view that takes the pointer at x, y in the
// surface: the last placed, as the content painted it over the others,
// whose placement takes it.
func (s *surface) viewAt(x, y float64) *window {
	for i := len(s.placed) - 1; i >= 0; i-- {
		if p := &s.placed[i]; p.At(x, y) {
			if v, ok := p.View.(*window); ok && !v.closed {
				return v
			}
		}
	}
	return nil
}

// hasKeyboard reports whether the embedded view, or a view in it, is the first
// responder of its window.
func (w *window) hasKeyboard() bool {
	r := send(w.win, "firstResponder")
	return r != 0 && sendBool(r, "isKindOfClass:", uintptr(class("NSView"))) &&
		sendBool(r, "isDescendantOf:", uintptr(w.embeddedView))
}

// TabInto gives the embedded view the keyboard as the window's key view loop
// does.
func (w *window) TabInto(back bool) {
	s := w.host.surface
	if w.closed || s == nil {
		return
	}
	withPool(func() {
		if back {
			send(w.embeddedView, "setNextKeyView:", uintptr(s.view))
			send(w.win, "selectKeyViewPrecedingView:", uintptr(s.view))
		} else {
			send(s.view, "setNextKeyView:", uintptr(w.embeddedView))
			send(w.win, "selectKeyViewFollowingView:", uintptr(s.view))
		}
	})
}

// tabOut gives the content the keyboard back as Tab leaves an embedded view.
func (w *window) tabOut(back bool) {
	s := w.host.surface
	if s == nil || w.host.closed {
		return
	}
	send(w.win, "makeFirstResponder:", uintptr(s.view))
	ev := platform.SurfaceEvent{Kind: platform.ViewTabOut, Key: platform.KeyTab, View: w}
	if back {
		ev.Mods = platform.ModShift
	}
	s.send(ev)
}

// embeddedViewOf returns the embedded view that is view or holds it.
func embeddedViewOf(view id) *window {
	if view == 0 {
		return nil
	}
	for _, w := range theBackend.byNSWindow {
		for _, v := range w.embeddedViews {
			if sendBool(view, "isDescendantOf:", uintptr(v.embeddedView)) || view == v.clip || view == v.pane {
				return v
			}
		}
	}
	return nil
}

// closeEmbeddedView closes an embedded view: by Close, or as its window closes.
func (w *window) closeEmbeddedView() {
	if w.closed {
		return
	}
	w.closed = true
	withPool(func() {
		if w.hasKeyboard() && w.host.surface != nil {
			send(w.win, "makeFirstResponder:", uintptr(w.host.surface.view))
		}
		send(w.embeddedView, "removeFromSuperview")
		send(w.clip, "removeFromSuperview")
		release(w.embeddedView)
		release(w.mask)
		release(w.pane)
		release(w.clip)
	})
	h := w.host
	h.embeddedViews = slices.DeleteFunc(h.embeddedViews, func(v *window) bool { return v == w })
	if s := h.surface; s != nil {
		s.placed = slices.DeleteFunc(s.placed, func(p platform.ViewPlacement) bool { return p.View == platform.EmbeddedView(w) })
	}
}

// closeEmbeddedViews closes the embedded views of a window that closes.
func (w *window) closeEmbeddedViews() {
	for len(w.embeddedViews) > 0 {
		w.embeddedViews[len(w.embeddedViews)-1].closeEmbeddedView()
	}
}

// unhandledKey gives the content a key that an embedded view in the clip view
// clip left unhandled, and reports whether it did.
func unhandledKey(clip, ev id) bool {
	w := clipEmbeddedView(clip)
	if w == nil {
		return false
	}
	k := eventKey(ev)
	if k == platform.KeyTab {
		return false
	}
	w.host.surface.send(platform.SurfaceEvent{Kind: platform.ViewKey, Key: k, PhysicalKey: physicalKey(ev), Mods: eventMods(ev), Repeat: sendBool(ev, "isARepeat"), View: w})
	return true
}

// unhandledCommand gives the content the key k of a command an embedded view in
// the clip view clip left, and reports whether it did.
func unhandledCommand(clip id, k platform.Key) bool {
	w := clipEmbeddedView(clip)
	if w == nil {
		return false
	}
	w.host.surface.send(platform.SurfaceEvent{Kind: platform.ViewKey, Key: k, View: w})
	return true
}

// clipEmbeddedView returns the embedded view in clip or pane.
func clipEmbeddedView(clip id) *window {
	for _, hw := range theBackend.byNSWindow {
		for _, v := range hw.embeddedViews {
			if v.clip == clip || v.pane == clip {
				if v.host == nil || v.host.closed || v.host.surface == nil {
					return nil
				}
				return v
			}
		}
	}
	return nil
}

func registerClipViewClass() {
	classDef("MyGoClipView", "NSView", nil, []objc.MethodDef{
		method("isFlipped", func(self id, _ objc.SEL) bool { return true }),
		method("keyDown:", func(self id, cmd objc.SEL, ev id) {
			if !unhandledKey(self, ev) {
				sendSuper(self, "MyGoClipView", cmd, uintptr(ev))
			}
		}),
		method("cancelOperation:", func(self id, cmd objc.SEL, sender id) {
			if !unhandledCommand(self, platform.KeyEscape) {
				if next := send(self, "nextResponder"); next != 0 {
					send(next, "doCommandBySelector:", uintptr(cmd))
				}
			}
		}),
		method("accessibilityChildren", func(self id, _ objc.SEL) id { return nsArray() }),
	})
}
