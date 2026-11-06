package ui

import (
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/scene"
)

// UILayer is a native UI engine rendered off-window into a reusable
// bitmap. It has its own layout, focus, input method, scrolling and
// accessibility state, so the same tree can be presented by a window, a
// WebGPU surface, or another compositor.
//
// Render and HandleInput run on the main thread. Render builds at most
// one frame; call NeedsFrame to see whether the view asked for another.
// The bitmap's scene is a snapshot taken by Render, so a later Render
// does not mutate the texture currently being presented.
type UILayer struct {
	rt             *engine
	host           *layerHost
	img            *scene.Image
	bm             *Bitmap
	closed         bool
	surfaceButtons uint32
	surfaceInside  bool
	lastUV         UV
	version        uint64
}

// NewUILayer creates a native UI layer of width×height DIPs and renders
// its first frame.
func NewUILayer(view func(*Context), width, height int) *UILayer {
	return NewUILayerWithScale(view, width, height, 1)
}

// NewUILayerWithScale is NewUILayer at scale device pixels per DIP. The
// layer's input and Size remain in DIPs; Bitmap dimensions are device
// pixels.
func NewUILayerWithScale(view func(*Context), width, height int, scale float32) *UILayer {
	width, height = max(width, 1), max(height, 1)
	if scale <= 0 {
		scale = 1
	}
	h := &headless{w: float32(width), h: float32(height), scale: scale}
	lh := &layerHost{headless: h}
	rt := newRuntime(func(c *context) { view(makeContext(c)) }, lh)
	pixelW, pixelH := max(int(float32(width)*scale), 1), max(int(float32(height)*scale), 1)
	img := scene.NewCanvas(pixelW, pixelH)
	l := &UILayer{rt: rt, host: lh, img: img, bm: &Bitmap{img: img, w: pixelW, h: pixelH}}
	l.Render()
	return l
}

type layerHost struct {
	*headless
	scene *scene.Scene
}

func (h *layerHost) present(s *scene.Scene) { h.scene = s }

// Bitmap returns the layer's bitmap for Image, Painter.Image,
// WebGPUCanvas.SetSource, and ImageRegion.
func (l *UILayer) Bitmap() *Bitmap { return l.bm }

// Size returns the layer's size in DIPs.
func (l *UILayer) Size() (w, h int) {
	return int(l.host.w), int(l.host.h)
}

// Scale returns the layer's device pixels per DIP.
func (l *UILayer) Scale() float32 { return l.host.scale }

// SetScale changes the layer's backing density without changing its DIP
// size or input coordinates. It asks for a frame, which the host renders
// as usual.
func (l *UILayer) SetScale(scale float32) {
	if l.closed {
		return
	}
	if scale <= 0 {
		scale = 1
	}
	if l.host.scale == scale {
		return
	}
	l.host.scale = scale
	l.host.invalidate()
}

// Render builds and paints one frame. It does nothing after Close.
func (l *UILayer) Render() {
	if l.closed || l.rt.closed {
		return
	}
	l.host.requested.Store(false)
	l.host.scene = nil
	l.rt.runFrame()
	if l.host.scene == nil || l.rt.scene.Width <= 0 || l.rt.scene.Height <= 0 {
		return
	}
	snapshot := snapshotScene(l.host.scene)
	l.img.Scene = snapshot
	l.img.W, l.img.H = snapshot.Width, snapshot.Height
	l.bm.w, l.bm.h = l.img.W, l.img.H
	l.img.Changed()
	l.version++
}

// Version identifies the rendered layer snapshot. It is independent of
// the SurfaceMap geometry version: a scene host presents one UI snapshot
// with one surface snapshot.
func (l *UILayer) Version() uint64 { return l.version }

// NeedsFrame reports whether the view asked for another frame.
func (l *UILayer) NeedsFrame() bool {
	return !l.closed && l.host.requested.Load()
}

// Invalidate asks for the layer to be rendered again.
func (l *UILayer) Invalidate() {
	if !l.closed {
		l.host.invalidate()
	}
}

// Resize changes the layer's logical size. It renders a new frame.
func (l *UILayer) Resize(width, height int) {
	if l.closed {
		return
	}
	width, height = max(width, 1), max(height, 1)
	l.host.w, l.host.h = float32(width), float32(height)
	l.host.invalidated.Store(true)
	l.Render()
}

// HandleInput sends an input event in the layer's DIP coordinates to the
// layer's engine. Pointer events use the same capture, hover, focus and
// IME rules as input sent to a native window.
func (l *UILayer) HandleInput(ev InputEvent) bool {
	if l.closed {
		return false
	}
	pe := platform.SurfaceEvent{
		X: float64(ev.X), Y: float64(ev.Y), Button: ev.Button, Clicks: ev.Clicks,
		DX: float64(ev.DX), DY: float64(ev.DY), Precise: ev.Precise, Momentum: ev.Momentum,
		Scale: float64(ev.Scale), Key: platform.Key(ev.Key), PhysicalKey: platform.Key(ev.PhysicalKey),
		Code: platform.KeyCode(ev.Code), Mods: platform.Modifiers(ev.Mods), Repeat: ev.Repeat,
		Text: ev.Text, Caret: ev.Caret,
		Pointer: platform.PointerKind(ev.Pointer), Pressure: float64(ev.Pressure),
	}
	switch ev.Kind {
	case InputPointerMove:
		pe.Kind = platform.PointerMove
	case InputPointerDown:
		pe.Kind = platform.PointerDown
	case InputPointerUp:
		if ev.Canceled {
			pe.Kind = platform.PointerCancel
		} else {
			pe.Kind = platform.PointerUp
		}
	case InputPointerLeave:
		pe.Kind = platform.PointerLeave
	case InputScroll:
		pe.Kind = platform.PointerScroll
	case InputZoom:
		pe.Kind = platform.PointerZoom
	case InputKeyDown:
		pe.Kind = platform.KeyPressed
	case InputKeyUp:
		pe.Kind = platform.KeyReleased
	case InputText:
		pe.Kind = platform.TextInput
	case InputCompose:
		pe.Kind = platform.TextComposition
	case InputCommand:
		pe.Kind = platform.SurfaceCommand
	default:
		return false
	}
	return l.rt.event(pe)
}

// HandleSurfaceInput maps a pointer or wheel event from the window into
// the layer through m, then sends it to the layer engine. While a press
// is held, a move that leaves the surface keeps using the last mapped
// layer coordinate, so pointer capture survives deforming or clipping
// edges. A move back onto the surface resumes tracking its UVs. Keyboard
// and text events pass through without mapping.
func (l *UILayer) HandleSurfaceInput(m SurfaceMap, ev InputEvent) bool {
	if l.closed {
		return false
	}
	switch ev.Kind {
	case InputKeyDown, InputKeyUp, InputText, InputCompose, InputCommand:
		return l.HandleInput(ev)
	case InputPointerLeave:
		l.surfaceInside = false
		return l.HandleInput(ev)
	}
	pressed := l.surfaceButtons != 0
	if m == nil {
		if !pressed {
			return false
		}
		ev.X = l.lastUV.U * l.host.w
		ev.Y = l.lastUV.V * l.host.h
		return l.finishSurfaceInput(l.HandleInput(ev), ev)
	}
	hit, ok := m.ScreenToSurface(Point{X: ev.X, Y: ev.Y})
	if ok && hit.Backface {
		ok = false
	}
	if !ok && !pressed {
		if l.surfaceInside {
			l.surfaceInside = false
			return l.HandleInput(InputEvent{Kind: InputPointerLeave})
		}
		return false
	}
	if ok {
		l.lastUV = hit.UV
		l.surfaceInside = true
	}
	ev.X = l.lastUV.U * l.host.w
	ev.Y = l.lastUV.V * l.host.h
	taken := l.HandleInput(ev)
	if ev.Kind == InputPointerDown && l.rt.pressed != nil {
		// PointerDown itself has no "handled" result in the native input
		// stream. A target holding the press is what tells a presenting
		// camera or scene not to take the same gesture.
		taken = true
	}
	return l.finishSurfaceInput(taken, ev)
}

func (l *UILayer) finishSurfaceInput(taken bool, ev InputEvent) bool {
	if ev.Kind != InputPointerDown && ev.Kind != InputPointerUp {
		return taken
	}
	button := ev.Button
	if button < 0 || button >= 32 {
		return taken
	}
	if ev.Kind == InputPointerDown {
		l.surfaceButtons |= 1 << uint(button)
	} else if ev.Canceled {
		l.surfaceButtons = 0
	} else {
		l.surfaceButtons &^= 1 << uint(button)
	}
	return taken
}

// Cursor returns the pointer shape the layer's hovered or focused native
// elements ask the presenting surface to show.
func (l *UILayer) Cursor() Cursor {
	if l.closed {
		return CursorDefault
	}
	return l.host.cursor
}

// HostLayer makes e the native host of layer's text input, cursor and
// accessibility tree through surface. It does not route pointer input or
// render the layer: use layer.HandleSurfaceInput in e's HandleInput and
// render when layer.NeedsFrame reports true.
//
// The merged accessibility nodes carry layer-relative bounds projected
// through surface and then through e's layout and visual transforms.
// Assistive-technology actions route back to the layer's own engine.
func (e *node) HostLayer(layer *UILayer, surface SurfaceMap) *node {
	if e == nil || layer == nil || layer.closed || layer.rt.closed || surface == nil {
		return e
	}
	e.layer, e.surface = layer, surface
	e.layerVer = layer.Version()
	e.surfaceVer = surface.Version()
	e.flags |= flagFocusable
	active, _ := layer.TextInputActive()
	if active {
		e.textClient = layer.TextInputClient(surface)
		e.flags |= flagHover
	}
	if cursor := layer.Cursor(); cursor != CursorDefault {
		e.Cursor(cursor)
	} else if active {
		e.Cursor(CursorText)
	}
	return e
}

// HandleLayerInput sends ev to the layer bound to r with the geometry
// versions that binding last built. Pointer, wheel and leave events are
// refused if either the UI snapshot or surface map changed after that
// build, so a camera or simulation tick cannot resolve input against pixels
// that have not been presented. Keyboard and text events still pass
// through. Render the layer and invalidate its presenters when NeedsFrame
// reports true, as with HandleSurfaceInput.
func (r Handle) HandleLayerInput(c *Context, ev InputEvent) bool {
	rt := c.runtime()
	b := (&r).binding(rt, false)
	if b == nil || b.closed || b.id == 0 {
		return false
	}
	s := rt.states[b.id]
	if s == nil || s.layer == nil || s.surface == nil ||
		s.layer.closed || s.layer.rt.closed {
		return false
	}
	switch ev.Kind {
	case InputPointerDown, InputPointerMove, InputPointerUp,
		InputScroll, InputZoom, InputPointerLeave:
		if s.surface.Version() != s.surfaceVer || s.layer.Version() != s.layerVer {
			return false
		}
	}
	return s.layer.HandleSurfaceInput(s.surface, ev)
}

// Access returns what assistive technology sees of the layer's last
// frame, by accessible name, as Tester.Access does.
func (l *UILayer) Access(name string) (AccessInfo, bool) {
	if l.closed {
		return AccessInfo{}, false
	}
	return rtAccess(l.rt, func(_ Role, n *platform.AccessNode) bool { return n.Label == name })
}

// AccessByRole returns what assistive technology sees of the layer's last
// frame, by role and accessible name, as Tester.AccessByRole does.
func (l *UILayer) AccessByRole(r Role, name string) (AccessInfo, bool) {
	if l.closed {
		return AccessInfo{}, false
	}
	return rtAccess(l.rt, func(role Role, n *platform.AccessNode) bool {
		return role == r && (name == "" || n.Label == name)
	})
}

// TextCaret returns where an input method composes in the layer, and
// whether one would, as Tester.TextCaret does.
func (l *UILayer) TextCaret() (Rect, bool) {
	c := l.host.ime.Caret
	return Rect{float32(c.X), float32(c.Y), float32(c.W), float32(c.H)},
		l.host.ime.Active && !l.host.ime.NoInputMethod
}

// TextInputActive reports whether the layer's focused element takes text
// and whether it turned input methods off.
func (l *UILayer) TextInputActive() (active, noInputMethod bool) {
	return l.host.ime.Active, l.host.ime.NoInputMethod
}

// TextInputClient bridges the layer's focused text input to the native
// window that presents it. The returned client translates input-method
// geometry through m, which is in the presenting element's coordinate
// space, and forwards composition and editing to the layer. Keep the
// returned value while the layer is presented and pass it to the
// presenting element's HandleTextInput.
//
// The client captures the mapping's geometry version. Caret bounds and
// point queries report unavailable if the map advances without a new
// client, so text services never target geometry newer than the frame
// that created them. Build the client in the same pass as the presenting
// element. A nil mapping returns nil.
func (l *UILayer) TextInputClient(m SurfaceMap) TextInputClient {
	if l.closed || m == nil {
		return nil
	}
	return &layerTextClient{layer: l, surface: m, version: m.Version()}
}

// Close drops the layer's engine and rendered resources.
func (l *UILayer) Close() {
	if l.closed {
		return
	}
	l.closed = true
	l.rt.close()
	l.host.scene = nil
	l.img.Scene = nil
	l.img.Pix = nil
	l.img.Changed()
}

// snapshotScene copies the display-list headers a layer keeps after its
// engine reuses the live scene on the next Render.
func snapshotScene(s *scene.Scene) *scene.Scene {
	out := *s
	out.Ops = append([]scene.Op(nil), s.Ops...)
	out.Glyphs = append([]scene.Glyph(nil), s.Glyphs...)
	out.Stops = append([]scene.Stop(nil), s.Stops...)
	out.Effects = append([]scene.EffectOp(nil), s.Effects...)
	out.Words = append([]uint32(nil), s.Words...)
	out.Wide = append([]scene.WideColors(nil), s.Wide...)
	out.Canvases = append([]*scene.Image(nil), s.Canvases...)
	return &out
}
