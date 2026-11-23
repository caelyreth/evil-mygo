//go:build linux && !android && (amd64 || arm64)

package linux

import (
	"slices"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/egoist/mygo/internal/platform"
)

// GDK window state flags.
const (
	stateIconified  = 1 << 1
	stateMaximized  = 1 << 2
	stateFullscreen = 1 << 4
	stateAbove      = 1 << 5
	stateFocused    = 1 << 7
	stateTiled      = 1 << 8
	// The edges the window manager lets resize (GDK 3.22.23 and later).
	stateTopResizable    = 1 << 10
	stateRightResizable  = 1 << 12
	stateBottomResizable = 1 << 14
	stateLeftResizable   = 1 << 16
)

type window struct {
	b    *Backend
	id   int
	h    platform.WindowHandler
	opts *platform.WindowOptions

	win     ptr // GtkWindow
	box     ptr // GtkBox holding the menu bar and the content surface
	menubar ptr
	accel   ptr
	owner   int
	ownMenu bool
	// autoHideMenu shows the menu bar only while its menus are open;
	// altAlone is an Alt press no other key or click has joined.
	autoHideMenu, altAlone bool
	// surface shows the content MyGo draws.
	surface *surface
	// controls are the title buttons over the surface of a window with a
	// hidden title bar (titlebar.go), in an overlay with the surface.
	controls *windowControls
	// csd tells that GTK decorates a frameless window or one with a hidden
	// title bar itself, with an empty title bar: it draws the shadow and
	// the border, and resizes the window at its edges (see useCSD).
	csd bool

	// flyout is how a flyout (WindowOptions.Flyout) was placed last, at
	// flyoutSize, once placed is set. grabbed tells that a focusable one
	// grabs the input, dismissing that it asks whether to close; unmaps
	// counts the unmap events its hiding has yet to bring.
	flyout                      platform.Flyout
	flyoutSize                  platform.Size
	placed, grabbed, dismissing bool
	unmaps                      int
	// layer tells that a panel is a layer surface (layer.go), put at
	// layerAt, or centered by the compositor without it.
	layer   bool
	layerAt *platform.Point
	// overlay tells that the layer surface is an always-on-top window
	// (initOverlay), which never takes the keyboard by itself.
	overlay bool

	closed       bool
	programmatic bool
	keepAbove    bool
	// ignoreMouse passes pointer events through the window to whatever is
	// beneath it (SetIgnoreMouseEvents), and inputRegion through all of
	// it but its rectangles, in DIPs from the top-left corner of the
	// content (SetInputRegion), nil for none; ignoreMouse wins. allocWatched
	// tells that the window's size-allocate shapes it again.
	ignoreMouse  bool
	inputRegion  []platform.Rect
	allocWatched bool
	// hiding tells that the window hides once the frame that asked for
	// that returns (Hide), unless it shows again first.
	hiding bool
	// requested holds bounds set with SetBounds until the window manager
	// answers: GTK resizes and moves asynchronously. requestFrame is the
	// window manager's frame they were asked for with, and requestInside
	// tells that the size asked for is the one inside it (as with
	// SetContentBounds). previous is the size GTK gave the window then,
	// which reports sent before still carry. placing is set while the
	// window manager places a window that shows.
	requested     *platform.Rect
	requestFrame  extents
	requestInside bool
	previous      [2]int32
	placing       bool
	// framed tells that the window manager framed the window that shows,
	// and frame is its frame as last seen, once seenFrame is set (see
	// frameExtents).
	framed, seenFrame bool
	frame             extents
	state             uint32
	x, y, w, hgt      int32
	minW, minH        int32
	maxW, maxH        int32

	// activationResize holds the latest bounds asked for while a window
	// drawing with GL on Wayland is not focused, and activationPrevious the
	// size it had before: the configure event of its activation can bring
	// that size back. resizeIdle is the idle source that then asks for the
	// bounds again (cbActivationResize).
	activationResize   *platform.Rect
	activationPrevious [2]int32
	resizeIdle         uint32

	// The last button press, used for window dragging and context menu
	// positioning.
	press struct {
		button     int32
		rootX      float64
		rootY      float64
		time       uint32
		event      ptr
		hasPressed bool
	}

	// dragPending holds whether a window drag was requested with the last
	// press and waits for the pointer to move; see StartDrag.
	dragPending bool
	// layerMove is the move or resize of a layer surface by the pointer,
	// while its button stays down (moveLayer).
	layerMove *layerMove

	// cursor is the resize cursor shown over an edge of a frameless
	// window, and the page's cursor it replaced.
	cursor struct {
		on    bool
		edge  int32
		shown ptr
		saved ptr // a reference
	}
}

func (b *Backend) window(data ptr) *window {
	w := b.windows[int(data)]
	if w == nil || w.closed {
		return nil
	}
	return w
}

func (b *Backend) NewWindow(o *platform.WindowOptions, h platform.WindowHandler) (platform.Window, error) {
	b.nextID++
	w := &window{b: b, id: b.nextID, h: h, opts: o, autoHideMenu: o.AutoHideMenu}
	data := ptr(w.id)

	typ := int32(0) // GTK_WINDOW_TOPLEVEL
	if o.Flyout != nil && o.Flyout.Tray == nil {
		typ = 1 // GTK_WINDOW_POPUP
	}
	w.win = gtkWindowNew(typ)
	gtkWindowSetTitle(w.win, cs(o.Title))
	if o.Panel {
		w.initLayer() // before anything realizes the window
	} else if overlayWindow(o) {
		w.initOverlay()
	}
	if o.Center {
		gtkWindowSetPosition(w.win, 1) // GTK_WIN_POS_CENTER
	} else {
		gtkWindowMove(w.win, int32(o.X), int32(o.Y))
	}
	gtkWindowSetResizable(w.win, o.Resizable)
	gtkWindowSetDeletable(w.win, o.Closable)
	if w.undecorated() {
		if w.useCSD() {
			w.decorate()
		} else {
			gtkWindowSetDecorated(w.win, false)
		}
	}
	if o.AlwaysOnTop {
		w.SetAlwaysOnTop(true)
	}
	if o.SkipTaskbar {
		gtkWindowSetSkipTaskbarHint(w.win, true)
	}
	if o.Opacity > 0 && o.Opacity < 1 {
		gtkWidgetSetOpacity(w.win, o.Opacity)
	}
	w.minW, w.minH = int32(o.MinSize.Width), int32(o.MinSize.Height)
	w.maxW, w.maxH = int32(o.MaxSize.Width), int32(o.MaxSize.Height)
	w.applyGeometry()
	if o.Transparent {
		if visual := gdkScreenGetRGBAVisual(gtkWidgetGetScreen(w.win)); visual != 0 {
			gtkWidgetSetVisual(w.win, visual)
		}
		gtkWidgetSetAppPaintable(w.win, true)
	}
	if p, ok := o.Parent.(*window); ok && p != nil {
		gtkWindowSetTransientFor(w.win, p.win)
		if o.Modal {
			gtkWindowSetModal(w.win, true)
		}
	}

	w.box = gtkBoxNew(1, 0) // vertical
	// GTK makes a window whose child has no natural size, as the web view,
	// 200x200 by nature, and one the user cannot resize never smaller than
	// its natural size.
	gtkWidgetSetSizeRequest(w.box, 1, 1)
	gtkContainerAdd(w.win, w.box)
	if w.hiddenTitleBar() && !o.HideWindowButtons {
		w.newControls()
	}
	w.createSurface()
	if w.controls != nil {
		gtkContainerAdd(w.controls.overlay, w.contentWidget())
	} else {
		gtkBoxPackStart(w.box, w.contentWidget(), true, true, 0)
	}
	w.accel = gtkAccelGroupNew()
	gtkWindowAddAccelGroup(w.win, w.accel)
	if b.appMenu != nil && w.hasAppMenu() {
		w.installMenu(b.appMenu)
	}
	// GTK sizes the window inside the window manager's frame, with the menu
	// bar.
	width, height := int32(o.Width), int32(o.Height)
	if o.UseContentSize {
		in := w.contentInsets()
		width, height = width+in.left+in.right, height+in.top+in.bottom
	} else {
		f := w.frameExtents()
		width, height = width-f.left-f.right, height-f.top-f.bottom
	}
	gtkWindowSetDefaultSize(w.win, max(width, 1), max(height, 1))
	if w.layer {
		w.sizeLayer(max(width, 1), max(height, 1))
	}

	connect(w.win, "delete-event", cbDeleteEvent, data)
	connect(w.win, "destroy", cbDestroy, data)
	connect(w.win, "focus-in-event", cbFocusIn, data)
	connect(w.win, "focus-out-event", cbFocusOut, data)
	connect(w.win, "configure-event", cbConfigure, data)
	connect(w.win, "window-state-event", cbWindowState, data)
	connect(w.win, "key-press-event", cbMenuKey, data)
	connect(w.win, "key-release-event", cbMenuKey, data)
	connect(w.win, "realize", cbWindowRealize, data)
	connectAfter(w.win, "map", cbWindowMap, data)

	b.windows[w.id] = w
	if (o.Frameless || w.hiddenTitleBar() && !b.compositorDecorates) && !w.csd && o.Flyout == nil && !w.layer && b.announceCSD != nil {
		// GTK asks a Wayland compositor that speaks
		// org_kde_kwin_server_decoration (KWin, COSMIC, Sway…) to decorate
		// every window it does not decorate itself, an undecorated one
		// too: say the window decorates itself, before it is mapped, so
		// that the compositor draws no title bar. A hidden title bar keeps
		// a tiling compositor's decorations (titlebar.go).
		gtkWidgetRealize(w.win)
		b.announceCSD(gtkWidgetGetWindow(w.win))
	}
	gtkWidgetShowAll(w.box)
	if o.Flyout != nil {
		w.initFlyout()
	}
	if o.Panel {
		w.initPanel()
	}
	if o.FullScreen {
		gtkWindowFullscreen(w.win)
	} else if o.Maximized {
		gtkWindowMaximize(w.win)
	}
	return w, nil
}

func (w *window) applyGeometry() {
	g := gdkGeometry{MinWidth: w.minW, MinHeight: w.minH, MaxWidth: w.maxW, MaxHeight: w.maxH}
	mask := int32(0)
	if w.minW > 0 || w.minH > 0 {
		mask |= 1 << 1 // GDK_HINT_MIN_SIZE
	}
	if w.maxW > 0 || w.maxH > 0 {
		if g.MaxWidth == 0 {
			g.MaxWidth = 1 << 20
		}
		if g.MaxHeight == 0 {
			g.MaxHeight = 1 << 20
		}
		mask |= 1 << 2 // GDK_HINT_MAX_SIZE
	}
	gtkWindowSetGeometryHints(w.win, 0, &g, mask)
}

func (w *window) cleanup() {
	if w.closed {
		return
	}
	w.closed = true
	delete(w.b.windows, w.id)
	w.leaveFlyout()
	w.b.flyouts = slices.DeleteFunc(w.b.flyouts, func(x *window) bool { return x == w })
	dropOwner(w.owner)
	if w.accel != 0 {
		// The window gives up its own reference as GTK destroys it; this
		// is the one gtk_accel_group_new returned, as installMenu frees
		// the group it replaces.
		delete(roleKeys, w.accel)
		gObjectUnref(w.accel)
		w.accel = 0
	}
	if w.surface != nil {
		w.surface.destroy()
	}
	if w.press.event != 0 {
		gdkEventFree(w.press.event)
		w.press.event = 0
	}
	w.releaseCursor()
}

func (w *window) Handle() uintptr { return w.win }

func (w *window) SetTitle(title string) { gtkWindowSetTitle(w.win, cs(title)) }
func (w *window) Title() string         { return goStr(gtkWindowGetTitle(w.win)) }

// The core marks the title of an edited document; GTK shows no file.
func (w *window) SetRepresentedFile(string) {}
func (w *window) SetDocumentEdited(bool)    {}

// extents are the widths of the edges around a rectangle.
type extents struct{ left, top, right, bottom int32 }

// grow returns r with the extents around it.
func (e extents) grow(r platform.Rect) platform.Rect {
	return platform.Rect{
		X: r.X - int(e.left), Y: r.Y - int(e.top),
		Width: r.Width + int(e.left+e.right), Height: r.Height + int(e.top+e.bottom),
	}
}

// shrink returns r without the extents around its inside.
func (e extents) shrink(r platform.Rect) platform.Rect {
	return platform.Rect{
		X: r.X + int(e.left), Y: r.Y + int(e.top),
		Width: r.Width - int(e.left+e.right), Height: r.Height - int(e.top+e.bottom),
	}
}

// frameExtents returns the extents of the frame a reparenting window
// manager puts around the window on X11, title bar and borders: GTK sizes
// the window inside it and positions the frame (NORTH_WEST gravity). They
// are known once the window manager framed the window that shows, which
// its first own (synthetic) configure report tells. Until then the window
// expects the frame it had last, or the one seen last around a decorated
// window in its normal state that the user can resize or not as this one,
// or none. Wayland tells no positions, and its compositors' decorations
// have no say in GTK's sizes.
func (w *window) frameExtents() extents {
	if !w.b.onX11 {
		return extents{}
	}
	resizable := 0
	if gtkWindowGetResizable(w.win) {
		resizable = 1
	}
	if !w.framed || !gtkWidgetGetMapped(w.win) {
		switch {
		case w.seenFrame:
			return w.frame
		case w.undecorated():
			return extents{}
		}
		return w.b.frames[resizable]
	}
	gdkWin := gtkWidgetGetWindow(w.win)
	var frame gdkRectangle
	gdkWindowGetFrameExtents(gdkWin, &frame)
	var x, y, width, height int32
	gdkWindowGetOrigin(gdkWin, &x, &y)
	gdkWindowGetGeometry(gdkWin, nil, nil, &width, &height) // from the server, as the frame
	f := extents{x - frame.X, y - frame.Y, frame.X + frame.Width - x - width, frame.Y + frame.Height - y - height}
	w.frame, w.seenFrame = f, true
	const special = stateMaximized | stateFullscreen | stateTiled | stateIconified
	if !w.undecorated() && w.state&special == 0 {
		w.b.frames[resizable] = f
	}
	return f
}

// contentInsets returns where the content is inside the window GTK sizes:
// below the menu bar, which GTK gives its natural height. That holds
// before GTK lays out a menu bar that just showed or went, unlike where
// it has the content.
func (w *window) contentInsets() extents {
	if w.menubar == 0 || !gtkWidgetGetVisible(w.menubar) {
		return extents{}
	}
	var minimum, natural int32
	gtkWidgetGetPreferredHeight(w.menubar, &minimum, &natural)
	return extents{top: natural}
}

// SetBounds asks for the bounds of the window manager's frame: GTK moves
// the frame and sizes the window inside it.
func (w *window) SetBounds(r platform.Rect) {
	if w.layer {
		w.setLayerBounds(r)
		return
	}
	w.setBounds(r, false)
}

func (w *window) SetContentBounds(r platform.Rect) {
	r = w.contentInsets().grow(r)
	if w.layer {
		w.setLayerBounds(r) // no frame around it
		return
	}
	w.setBounds(w.frameExtents().grow(r), true)
}

// setBounds asks for the bounds of the frame, for the size inside it when
// inside is set: a frame other than expected keeps that size (see
// cbConfigure).
func (w *window) setBounds(r platform.Rect, inside bool) {
	f := w.frameExtents()
	width, height := w.constrain(int32(r.Width)-f.left-f.right, int32(r.Height)-f.top-f.bottom)
	r.Width, r.Height = int(width+f.left+f.right), int(height+f.top+f.bottom)
	if w.requested == nil {
		gtkWindowGetSize(w.win, &w.previous[0], &w.previous[1])
	}
	w.requested, w.requestFrame, w.requestInside = &r, f, inside
	if w.resizeIdle != 0 {
		w.activationResize = &r // the idle callback asks for it
		return
	}
	if !w.b.onX11 && w.surface != nil && w.surface.rendered && w.surface.drawsGL() && w.state&stateFocused == 0 {
		if w.activationResize == nil {
			gtkWindowGetSize(w.win, &w.activationPrevious[0], &w.activationPrevious[1])
		}
		w.activationResize = &r
	}
	// GTK keeps a window the user cannot resize at least as large as its
	// default size.
	gtkWindowSetDefaultSize(w.win, width, height)
	gtkWindowMove(w.win, int32(r.X), int32(r.Y))
	gtkWindowResize(w.win, width, height)
}

// constrain returns the size GTK gives the window when asked for one:
// within its minimum and maximum sizes, and no smaller than its content, or
// than the content's natural size when the user cannot resize the window.
func (w *window) constrain(width, height int32) (int32, int32) {
	var minW, natW, minH, natH int32
	gtkWidgetGetPreferredWidth(w.box, &minW, &natW)
	gtkWidgetGetPreferredHeight(w.box, &minH, &natH)
	if !gtkWindowGetResizable(w.win) {
		minW, minH = natW, natH
	}
	if w.maxW > 0 {
		width = min(width, w.maxW)
	}
	if w.maxH > 0 {
		height = min(height, w.maxH)
	}
	return max(width, w.minW, minW), max(height, w.minH, minH)
}

// Bounds returns the bounds of the window manager's frame, which are the
// window's own without one.
func (w *window) Bounds() platform.Rect {
	if w.layer {
		return w.layerBounds()
	}
	if w.requested != nil {
		return *w.requested
	}
	if w.framed && gtkWidgetGetMapped(w.win) {
		var r gdkRectangle
		gdkWindowGetFrameExtents(gtkWidgetGetWindow(w.win), &r)
		return platform.Rect{X: int(r.X), Y: int(r.Y), Width: int(r.Width), Height: int(r.Height)}
	}
	var x, y, width, height int32
	gtkWindowGetPosition(w.win, &x, &y) // the frame's
	gtkWindowGetSize(w.win, &width, &height)
	f := w.frameExtents()
	return platform.Rect{X: int(x), Y: int(y), Width: int(width + f.left + f.right), Height: int(height + f.top + f.bottom)}
}

// size returns the size GTK gives the window, or was asked to give it.
func (w *window) size() (width, height int32) {
	if r := w.requested; r != nil {
		f := w.requestFrame
		return int32(r.Width) - f.left - f.right, int32(r.Height) - f.top - f.bottom
	}
	gtkWindowGetSize(w.win, &width, &height)
	return width, height
}

// ContentBounds returns where the content is: on X11, where X has the
// window inside the window manager's frame, below the menu bar.
func (w *window) ContentBounds() platform.Rect {
	var r platform.Rect
	if w.requested == nil && w.b.onX11 && gtkWidgetGetMapped(w.win) {
		var x, y int32
		gdkWindowGetOrigin(gtkWidgetGetWindow(w.win), &x, &y)
		width, height := w.size()
		r = platform.Rect{X: int(x), Y: int(y), Width: int(width), Height: int(height)}
	} else {
		f := w.requestFrame
		if w.requested == nil {
			f = w.frameExtents()
		}
		r = f.shrink(w.Bounds())
	}
	return w.contentInsets().shrink(r)
}

func (w *window) SetMinimumSize(s platform.Size) {
	w.minW, w.minH = int32(s.Width), int32(s.Height)
	w.applyGeometry()
}

func (w *window) SetMaximumSize(s platform.Size) {
	w.maxW, w.maxH = int32(s.Width), int32(s.Height)
	w.applyGeometry()
}

func (w *window) SetResizable(v bool) {
	if !v {
		// Keep the size it has or was given (see SetBounds).
		width, height := w.size()
		gtkWindowSetDefaultSize(w.win, width, height)
	}
	gtkWindowSetResizable(w.win, v)
	if w.controls != nil {
		w.layoutControls() // with a maximize button or without
	}
}

func (w *window) IsResizable() bool   { return gtkWindowGetResizable(w.win) }
func (w *window) SetMovable(bool)     {}
func (w *window) IsMovable() bool     { return true }
func (w *window) SetMinimizable(bool) {}
func (w *window) IsMinimizable() bool { return !w.opts.Panel }
func (w *window) SetMaximizable(bool) {}
func (w *window) IsMaximizable() bool { return gtkWindowGetResizable(w.win) && !w.opts.Panel }
func (w *window) SetClosable(v bool) {
	gtkWindowSetDeletable(w.win, v)
	if w.controls != nil {
		w.layoutControls() // with a close button or without
	}
}

func (w *window) IsClosable() bool { return gtkWindowGetDeletable(w.win) }
func (w *window) SetAlwaysOnTop(v bool) {
	if w.overlay {
		// gtk-layer-shell turns no layer surface back into a window: it
		// stays above.
		return
	}
	w.keepAbove = v
	gtkWindowSetKeepAbove(w.win, v)
}

// IsAlwaysOnTop reports the requested state: only some window managers
// reflect it in the window state. An overlay on a layer surface is above
// for good.
func (w *window) IsAlwaysOnTop() bool { return w.overlay || w.keepAbove || w.state&stateAbove != 0 }

func (w *window) Show() {
	if w.opts.Flyout != nil {
		w.showFlyout(w.flyout.Focusable)
		return
	}
	if w.layer {
		w.layerKeyboard(true)
	}
	w.enterFlyout()
	w.willShow()
	gtkWidgetShow(w.win)
	gtkWindowPresent(w.win)
}

func (w *window) ShowInactive() {
	if w.opts.Flyout != nil {
		w.showFlyout(false)
		return
	}
	if w.layer {
		w.layerKeyboard(false)
	}
	w.enterFlyout()
	w.willShow()
	gtkWidgetShow(w.win)
}

// willShow makes Bounds report where GTK asks for a window about to show
// until the window manager put it there. Until then, X has the window where
// GTK created it or where it was, and a reparenting window manager has its
// frame where it created that. A window that shows again before a Hide
// its frame asked for stays shown.
func (w *window) willShow() {
	w.hiding = false
	if gtkWidgetGetVisible(w.win) {
		return
	}
	w.framed = false // until the window manager frames it again
	if w.requested == nil {
		b := w.Bounds() // what GTK asks for, while the window does not show
		// The size it was made with, or had, unless it was made with
		// that of its frame.
		inside := w.opts.UseContentSize || w.seenFrame
		w.requested, w.requestFrame, w.requestInside = &b, w.frameExtents(), inside
		w.previous[0], w.previous[1] = w.size()
	}
	w.placing = w.b.windowManager(w.win)
}

func (w *window) Hide() {
	if w.opts.Flyout != nil {
		w.hideFlyout()
		return
	}
	if s := w.surface; s != nil && s.drawing() {
		// Hiding releases the GL context of the area, which GTK still
		// paints once the frame returns (#680): hide the window after,
		// before the next events (G_PRIORITY_HIGH), as Close destroys it.
		if !w.hiding {
			w.hiding = true
			gIdleAddFull(-100, cbHideLater, ptr(w.id), 0)
		}
		return
	}
	w.hiding = false
	gtkWidgetHide(w.win)
	w.leaveFlyout()
}

func (w *window) IsVisible() bool { return gtkWidgetGetVisible(w.win) && !w.hiding }

func (w *window) Focus() {
	if w.opts.Flyout != nil {
		// A popup takes the keyboard only by its grab, as it shows.
		w.showFlyout(w.flyout.Focusable)
		return
	}
	w.enterFlyout()
	w.b.dismissFlyouts(w)
	if w.layer {
		w.layerKeyboard(true)
	}
	w.willShow() // presenting shows a hidden window
	gtkWindowPresent(w.win)
}

func (w *window) Blur()           {}
func (w *window) IsFocused() bool { return gtkWindowIsActive(w.win) }

// CornerRadius is 0: the window manager draws the corners, and GTK does
// not tell their radius.
func (w *window) CornerRadius() float64 { return 0 }

func (w *window) Minimize()         { gtkWindowIconify(w.win) }
func (w *window) IsMinimized() bool { return w.state&stateIconified != 0 }
func (w *window) Maximize()         { gtkWindowMaximize(w.win) }
func (w *window) Unmaximize()       { gtkWindowUnmaximize(w.win) }
func (w *window) IsMaximized() bool { return gtkWindowIsMaximized(w.win) }
func (w *window) Restore()          { gtkWindowDeiconify(w.win) }

func (w *window) SetFullScreen(v bool) {
	if v {
		gtkWindowFullscreen(w.win)
	} else {
		gtkWindowUnfullscreen(w.win)
	}
}

func (w *window) IsFullScreen() bool { return w.state&stateFullscreen != 0 }

func (w *window) SetProgressBar(state string, value float64) {
	l := &w.b.launcher
	l.showing = state != ""
	l.progress = value
	if state == "indeterminate" {
		l.progress = 1
	}
	w.b.updateLauncherEntry()
}

func (w *window) FlashFrame(flash bool) { gtkWindowSetUrgencyHint(w.win, flash) }
func (w *window) SetSkipTaskbar(v bool) { gtkWindowSetSkipTaskbarHint(w.win, v) }

func (w *window) SetVisibleOnAllWorkspaces(v bool) {
	if v {
		gtkWindowStick(w.win)
	} else {
		gtkWindowUnstick(w.win)
	}
}

func (w *window) SetIcon(png []byte) error {
	if png == nil {
		gtkWindowSetIcon(w.win, 0) // back to the default icon
		return nil
	}
	pix, err := pixbufFromPNG(png)
	if err != nil {
		return err
	}
	defer gObjectUnref(pix)
	gtkWindowSetIcon(w.win, pix)
	return nil
}

func (w *window) Center() {
	if w.layer {
		w.centerLayer()
		return
	}
	b := w.Bounds()
	for _, d := range (screen{}).Displays() {
		if d.Primary {
			area := d.WorkArea
			x, y := area.X+(area.Width-b.Width)/2, area.Y+(area.Height-b.Height)/2
			if w.requested != nil {
				// Bounds that GTK has not confirmed yet move with it.
				w.requested.X, w.requested.Y = x, y
			}
			gtkWindowMove(w.win, int32(x), int32(y))
			return
		}
	}
}

func (w *window) SetBackgroundColor(platform.Color) {}

func (w *window) SetOpacity(v float64)            { gtkWidgetSetOpacity(w.win, v) }
func (w *window) Opacity() float64                { return gtkWidgetGetOpacity(w.win) }
func (w *window) SetContentProtection(bool)       {}
func (w *window) SetVibrancy(string)              {}
func (w *window) SetVibrancyTint(*platform.Color) {}
func (w *window) SetActiveAppearance(bool)        {}

func (w *window) IgnoresMouseEvents() bool { return w.ignoreMouse }

func (w *window) SetIgnoreMouseEvents(v bool) {
	w.ignoreMouse = v
	w.applyInputShape(true)
}

// shapesInput tells that the window has an input shape of MyGo's.
func (w *window) shapesInput() bool { return w.ignoreMouse || w.inputRegion != nil }

// applyInputShape gives the window an input shape, which passes the
// pointer through to what is beneath outside it: an empty one to ignore
// the mouse, the input region's rectangles, or none, which removes it. A
// window not realized yet gets it as it realizes, and again as it maps.
// GTK gives a window it decorates itself an input shape of its own, to
// let the pointer through its shadow, as it allocates its size: the
// window's size-allocate, after GTK's, shapes it again, as it does the
// region where the content moved in the window, as under a menu bar
// that shows. Under Wayland the shape takes effect with the surface's
// next commit: a frame is drawn for it, when draw is set.
func (w *window) applyInputShape(draw bool) {
	if w.closed {
		return
	}
	if w.shapesInput() && !w.allocWatched {
		w.allocWatched = true
		connectAfter(w.win, "size-allocate", cbWindowAllocated, ptr(w.id))
	}
	win := gtkWidgetGetWindow(w.win)
	if win == 0 {
		return
	}
	loadInputRegion() // binds the GDK and cairo region functions
	loadSurface()  // and gtk_widget_queue_draw, for a window without native UI
	switch {
	case w.ignoreMouse:
		region := cairoRegionCreateRectangle(&gdkRectangle{})
		gdkWindowInputShapeCombineRegion(win, region, 0, 0)
		cairoRegionDestroy(region)
	case w.inputRegion != nil:
		shapeInputRegion(w, win)
	default:
		gdkWindowInputShapeCombineRegion(win, 0, 0, 0)
	}
	if draw {
		gtkWidgetQueueDraw(w.win)
	}
}

// SetWindowButtonsTheme does nothing: GTK's title buttons follow the
// theme, which is the application's.
func (w *window) SetWindowButtonsTheme(string) {}

// StartDrag begins moving the window with the last button press. On an
// undecorated window the compositor takes the pointer once the move
// starts, so the move waits until the pointer moves, keeping a second
// press free to reach the page as a double click. Decorated windows
// observe no motion on the page, so they move at once.
func (w *window) StartDrag() {
	if !w.press.hasPressed {
		return
	}
	if !w.undecorated() {
		gtkWindowBeginMoveDrag(w.win, w.press.button, int32(w.press.rootX), int32(w.press.rootY), w.press.time)
		return
	}
	w.dragPending = true
}

// checkDragStart starts a pending window drag once the pointer moves past
// the drag threshold with the button still down, and reports whether the
// pointer moves a layer surface, whose motion is not the page's.
func (w *window) checkDragStart(event ptr) bool {
	if !w.dragPending {
		return false
	}
	// GdkEventMotion: state 48, x_root 64, y_root 72.
	if field[uint32](event, 48)&(1<<8) == 0 {
		// The button went up without the pointer moving.
		w.dragPending = false
		return false
	}
	const dragThreshold = 8 // pixels, as GTK's
	dx, dy := field[float64](event, 64)-w.press.rootX, field[float64](event, 72)-w.press.rootY
	if dx*dx+dy*dy < dragThreshold*dragThreshold {
		return false
	}
	w.dragPending = false
	if w.layer {
		// The compositor moves toplevels only: a layer surface follows the
		// pointer itself.
		w.layerMove = &layerMove{edge: -1, start: w.layerBounds()}
		return w.moveLayer(event)
	}
	gtkWindowBeginMoveDrag(w.win, w.press.button, int32(w.press.rootX), int32(w.press.rootY), w.press.time)
	return false
}

// GTK gives windows without decorations no resize edges, so the outer
// pixels of the page resize frameless windows, as in Electron. Those GTK
// decorates itself (useCSD) resize at its edges, around the page.
const (
	resizeInset  = 5  // pixels of the page along the edges
	resizeCorner = 16 // pixels along the edges from a corner that resize it
)

// resizeEdges are the GdkWindowEdge values, with their cursors and the
// states in which the window manager lets them resize the window.
var resizeEdges = [8]struct {
	cursor string
	states uint32
}{
	{"nw-resize", stateTopResizable | stateLeftResizable},
	{"n-resize", stateTopResizable},
	{"ne-resize", stateTopResizable | stateRightResizable},
	{"w-resize", stateLeftResizable},
	{"e-resize", stateRightResizable},
	{"sw-resize", stateBottomResizable | stateLeftResizable},
	{"s-resize", stateBottomResizable},
	{"se-resize", stateBottomResizable | stateRightResizable},
}

// resizeEdge returns the GdkWindowEdge a pointer event on the page (a
// GdkEventMotion or GdkEventButton) would resize, or -1.
func (w *window) resizeEdge(event ptr) int32 {
	// GdkEventMotion and GdkEventButton: window 8, x 24, y 32.
	if !w.undecorated() || w.csd || w.state&(stateMaximized|stateFullscreen) != 0 ||
		field[ptr](event, 8) != w.contentWindow() || !gtkWindowGetResizable(w.win) {
		return -1
	}
	x, y := field[float64](event, 24), field[float64](event, 32)
	var page gdkRectangle
	gtkWidgetGetAllocation(w.contentWidget(), &page)
	width, height := float64(page.Width), float64(page.Height)
	atTop := page.Y == 0 // no menu bar above the page
	top, bottom := atTop && y < resizeInset, y >= height-resizeInset
	left, right := x < resizeInset, x >= width-resizeInset
	if !top && !bottom && !left && !right {
		return -1
	}
	// Near a corner, the edges resize the corner.
	top = top || (left || right) && atTop && y < resizeCorner
	bottom = bottom || (left || right) && y >= height-resizeCorner
	left = left || (top || bottom) && x < resizeCorner
	right = right || (top || bottom) && x >= width-resizeCorner
	var edge int32
	switch {
	case top && left:
		edge = 0
	case top && right:
		edge = 2
	case bottom && left:
		edge = 5
	case bottom && right:
		edge = 7
	case top:
		edge = 1
	case bottom:
		edge = 6
	case left:
		edge = 3
	default:
		edge = 4
	}
	// Tiled windows resize at the edges the window manager allows, as
	// with GTK's own decorations.
	const resizable = stateTopResizable | stateRightResizable | stateBottomResizable | stateLeftResizable
	if need := resizeEdges[edge].states; w.state&resizable != 0 && w.state&need != need ||
		w.state&resizable == 0 && w.state&stateTiled != 0 {
		return -1
	}
	return edge
}

// showResizeCursor shows the cursor of a resize edge over the page, or
// the page's own cursor again for -1.
func (w *window) showResizeCursor(edge int32) {
	c := &w.cursor
	if edge < 0 && !c.on {
		return
	}
	page := w.contentWindow()
	current := gdkWindowGetCursor(page)
	if edge < 0 {
		if c.on && current == c.shown {
			gdkWindowSetCursor(page, c.saved)
		}
		w.releaseCursor()
		return
	}
	if !c.on || current != c.shown {
		// The pointer comes onto an edge, or WebKit changed the cursor.
		w.releaseCursor()
		if current != 0 {
			gObjectRef(current)
		}
		c.on, c.edge, c.saved = true, -1, current
	}
	if c.edge != edge {
		cursor := gdkCursorNewFromName(gdkWindowGetDisplay(page), cs(resizeEdges[edge].cursor))
		gdkWindowSetCursor(page, cursor)
		if cursor != 0 {
			gObjectUnref(cursor) // the GdkWindow holds it
		}
		c.edge, c.shown = edge, cursor
	}
}

// releaseCursor forgets the page's cursor kept to restore.
func (w *window) releaseCursor() {
	if w.cursor.saved != 0 {
		gObjectUnref(w.cursor.saved)
	}
	w.cursor.on, w.cursor.edge, w.cursor.shown, w.cursor.saved = false, -1, 0, 0
}

// hiddenTitleBar reports a TitleBarStyle that hides the title bar (not a
// frameless window's, which has no buttons either).
func (w *window) hiddenTitleBar() bool {
	o := w.opts
	return !o.Frameless && (o.TitleBarStyle == "hidden" || o.TitleBarStyle == "hiddenInset")
}

// undecorated reports a window without the window manager's decorations,
// whose page's outer pixels resize it unless GTK decorates it (useCSD).
func (w *window) undecorated() bool { return w.opts.Frameless || w.hiddenTitleBar() }

// useCSD reports whether GTK decorates an undecorated window itself, as it
// does the windows of GNOME's apps: its client-side decorations, with an
// empty title bar, draw the shadow and the border around the page, and
// resize the window at its edges, where they are. That is on Wayland,
// whose compositors leave windows that decorate themselves without either.
// GTK sizes the window without them, so that the page keeps the size asked
// for, and drops them while the window is maximized or in full screen.
//
// Not decorated:
//   - Windows without a shadow (DisableShadow), as before.
//   - Transparent windows: the decorations would outline the window's
//     rectangle around a page that shows its own shape.
//   - Flyouts and panels: popups take no decorations, a panel on Wayland
//     is a layer surface, which the compositor sizes and puts without
//     them, and both have a look of their own (Flyout.Shadow).
//   - Overlays on a layer surface (initOverlay), which the compositor puts
//     without them too, and resizes through no edges of GTK's: the page's
//     edges resize them (moveLayer).
//   - A hidden title bar where a tiling compositor decorates windows
//     itself (Hyprland, Sway): it keeps what the compositor draws.
//   - X11, where Mutter puts its shadow around opaque undecorated windows
//     as it composites, and where a window that decorates itself has its
//     shadow inside its X window, which Bounds, SetBounds and the frame
//     extents of the window manager (frameExtents) do not count.
func (w *window) useCSD() bool {
	o := w.opts
	return !w.b.onX11 && o.HasShadow && !o.Transparent && o.Flyout == nil && !o.Panel && !w.layer &&
		!(w.hiddenTitleBar() && w.b.compositorDecorates)
}

// decorate has GTK decorate the window itself, before it is realized, with
// a title bar that never shows: GtkWindow lays out and draws no title bar
// that is not visible, and keeps its decorations.
func (w *window) decorate() {
	gtkWindowSetTitlebar(w.win, gtkBoxNew(0, 0))
	w.csd = true
}

// The shadow comes with the window (useCSD): GTK styles the decorations
// of a window that shows once, so that a style taking them away later
// changes nothing until the window is hidden and shown again, and then
// leaves the page smaller.
func (w *window) SetHasShadow(bool) {}

// HasShadow reports whether the window manager decorates the window, or GTK
// draws a shadow around it.
func (w *window) HasShadow() bool { return !w.undecorated() || w.csd }

func (w *window) TitleBarDoubleClicked() {
	if w.IsMaximized() {
		w.Unmaximize()
	} else {
		w.Maximize()
	}
}

func (w *window) Close() {
	if !w.closed {
		if s := w.surface; s != nil && s.drawing() {
			// GTK uses the window once the frame returns: destroy it after,
			// before the next events (G_PRIORITY_HIGH).
			gIdleAddFull(-100, cbCloseLater, ptr(w.id), 0)
			return
		}
		if w.opts.Flyout != nil {
			w.ungrab()
		}
		gtkWidgetDestroy(w.win)
	}
}

// Signal handlers. data carries the window id.
var (
	cbDeleteEvent, cbDestroy, cbFocusIn, cbFocusOut, cbConfigure, cbWindowState ptr
	cbButtonPress, cbMotion, cbControlsAllocated, cbDecorationLayout          ptr
	cbActivationResize, cbCloseLater, cbHideLater                               ptr
	cbWindowRealize, cbWindowMap, cbWindowAllocated                             ptr
)

func field[T any](p ptr, offset uintptr) T {
	return *(*T)(unsafe.Add(*(*unsafe.Pointer)(unsafe.Pointer(&p)), offset))
}

func initWindowCallbacks() {
	b := func() *Backend { return theBackend }
	cbDeleteEvent = purego.NewCallback(func(widget, event, data ptr) bool {
		if w := b().window(data); w != nil {
			return !w.h.ShouldClose()
		}
		return false
	})
	cbDestroy = purego.NewCallback(func(widget, data ptr) {
		if w := b().window(data); w != nil {
			w.cleanup()
			w.h.Closed()
		}
	})
	cbWindowRealize = purego.NewCallback(func(widget, data ptr) {
		if w := b().window(data); w != nil && w.shapesInput() {
			w.applyInputShape(true)
		}
	})
	cbWindowMap = cbWindowRealize
	cbWindowAllocated = purego.NewCallback(func(widget, allocation, data ptr) {
		if w := b().window(data); w != nil && w.shapesInput() {
			w.applyInputShape(false)
		}
	})
	cbCloseLater = purego.NewCallback(func(data ptr) int32 {
		if w := b().window(data); w != nil {
			w.Close()
		}
		return 0 // G_SOURCE_REMOVE
	})
	cbHideLater = purego.NewCallback(func(data ptr) int32 {
		if w := b().window(data); w != nil && w.hiding {
			w.hiding = false
			w.Hide()
		}
		return 0 // G_SOURCE_REMOVE
	})
	cbFocusIn = purego.NewCallback(func(widget, event, data ptr) bool {
		if w := b().window(data); w != nil {
			w.h.Focused()
		}
		return false
	})
	cbFocusOut = purego.NewCallback(func(widget, event, data ptr) bool {
		if w := b().window(data); w != nil {
			w.altAlone = false
			w.h.Blurred()
			if w.trayFlyout() && w.flyout.Focusable && !w.grabbingChild() && w.modalChild() == nil {
				w.dismissFlyout(false)
			}
		}
		return false
	})
	// A window drawing with GL on Wayland got focus: the configure event of
	// the activation can carry the size of the last buffer it drew, from
	// before a resize GTK already took, and GTK went back to it. Ask for the
	// latest bounds again then, unless the window manager sized the window.
	cbActivationResize = purego.NewCallback(func(data ptr) int32 {
		w := b().window(data)
		if w == nil {
			return 0
		}
		r := w.activationResize
		w.activationResize, w.resizeIdle = nil, 0
		var width, height int32
		gtkWindowGetSize(w.win, &width, &height)
		if w.state&(stateMaximized|stateFullscreen|stateTiled) == 0 &&
			(w.requested != nil || [2]int32{width, height} == w.activationPrevious) {
			// GTK skips a request for the size it asked for last: the first
			// check takes the configure, the second asks for the size the
			// window has, so that the bounds are asked for again. Handlers
			// of the new size may close the window.
			gtkWindowResize(w.win, width, height)
			gtkContainerCheckResize(w.win)
			if !w.closed {
				gtkContainerCheckResize(w.win)
			}
			if !w.closed {
				w.SetBounds(*r)
			}
		} else {
			w.requested = nil
		}
		return 0
	})
	cbConfigure = purego.NewCallback(func(widget, event, data ptr) bool {
		w := b().window(data)
		if w == nil {
			return false
		}
		// GdkEventConfigure: x, y, width, height at offsets 20..32.
		x, y := field[int32](event, 20), field[int32](event, 24)
		width, height := field[int32](event, 28), field[int32](event, 32)
		moved := x != w.x || y != w.y
		resized := width != w.w || height != w.hgt
		w.x, w.y, w.w, w.hgt = x, y, width, height
		// The window manager's own report (GdkEventConfigure's send_event,
		// at 16) comes once it framed and moved the window.
		synthetic := field[int8](event, 16) != 0
		if synthetic && w.b.onX11 {
			w.framed = true
		}
		if r := w.requested; r != nil && w.resizeIdle == 0 {
			// Without the decorations GTK draws, as requested.
			var cw, ch int32
			gtkWindowGetSize(w.win, &cw, &ch)
			f := w.frameExtents()
			if old := w.requestFrame; f != old {
				// The window manager framed the window otherwise than
				// expected as it placed it. GTK positions the frame, so
				// the bounds grow with the frame around the size asked
				// for inside it, or GTK is asked for the size inside the
				// frame again, which is done once GTK has it there.
				if w.requestInside {
					r.Width += int(f.left + f.right - old.left - old.right)
					r.Height += int(f.top + f.bottom - old.top - old.bottom)
					w.requestFrame = f
				} else {
					rw, rh := w.constrain(int32(r.Width)-f.left-f.right, int32(r.Height)-f.top-f.bottom)
					r.Width, r.Height = int(rw+f.left+f.right), int(rh+f.top+f.bottom)
					w.requestFrame, w.previous, w.placing = f, [2]int32{cw, ch}, false
					gtkWindowSetDefaultSize(w.win, rw, rh)
					gtkWindowResize(w.win, rw, rh)
				}
			}
			rw, rh := w.size()
			done := false
			switch {
			case cw != rw || ch != rh:
				if cw == w.previous[0] && ch == w.previous[1] {
					// A report from before the window manager took the
					// request, as openbox sends when the size hints change.
					// GTK would ask for this size again from it (and keeps a
					// window the user cannot resize as large), unless asked
					// for the new one again.
					gtkWindowResize(w.win, rw, rh)
				} else {
					done = true // the window manager, or the user, chose it
				}
			case synthetic || !w.b.onX11:
				done = true
			case !w.placing:
				// X11 reports where the window is, in the frame of the
				// window manager: it is done once that is where it was
				// asked to be.
				var cx, cy int32
				gtkWindowGetPosition(w.win, &cx, &cy)
				done = cx == int32(r.X) && cy == int32(r.Y)
			}
			if done {
				w.requested, w.placing = nil, false
			}
		}
		if resized {
			w.h.Resized()
		}
		if moved {
			w.h.Moved()
		}
		return false
	})
	cbWindowState = purego.NewCallback(func(widget, event, data ptr) bool {
		w := b().window(data)
		if w == nil {
			return false
		}
		// GdkEventWindowState: changed_mask at 20, new_window_state at 24.
		changed, state := field[uint32](event, 20), field[uint32](event, 24)
		w.state = state
		if changed&stateFocused != 0 && state&stateFocused != 0 && w.activationResize != nil && w.resizeIdle == 0 {
			// G_PRIORITY_DEFAULT_IDLE: after GTK took the configure event
			// of the activation.
			w.resizeIdle = gIdleAddFull(200, cbActivationResize, ptr(w.id), 0)
		}
		if changed&stateIconified != 0 {
			if state&stateIconified != 0 {
				w.h.Minimized()
			} else {
				w.h.Restored()
			}
			// Nothing of a minimized window shows (Surface.Occluded).
			if s := w.surface; s != nil {
				kind := platform.SurfaceShown
				if state&stateIconified != 0 {
					kind = platform.SurfaceHidden
				}
				s.send(platform.SurfaceEvent{Kind: kind})
			}
		}
		if changed&stateMaximized != 0 {
			if state&stateMaximized != 0 {
				w.h.Maximized()
			} else {
				w.h.Unmaximized()
			}
		}
		if changed&stateFullscreen != 0 {
			if state&stateFullscreen != 0 {
				w.h.EnteredFullScreen()
			} else {
				w.h.LeftFullScreen()
			}
			if w.menubar != 0 {
				gtkWidgetSetVisible(w.menubar, !w.menuHides())
			}
			if w.controls != nil {
				w.fullScreenChanged()
			}
		}
		return false
	})
	cbControlsAllocated = purego.NewCallback(func(widget, allocation, data ptr) {
		if w := b().window(data); w != nil && w.controls != nil {
			w.measureControls()
		}
	})
	// The desktop's button layout changed: every window's title buttons
	// follow it.
	cbDecorationLayout = purego.NewCallback(func(settings, pspec, data ptr) {
		for _, w := range b().windows {
			if w.controls != nil && !w.closed {
				w.layoutControls()
			}
		}
		// Theme.WindowButtons too.
		theBackend.h.ThemeChanged()
	})
	cbButtonPress = purego.NewCallback(func(widget, event, data ptr) bool {
		w := b().window(data)
		if w == nil {
			return false
		}
		w.altAlone = false
		// GdkEventButton: time 20, button 52, x_root 64, y_root 72.
		w.press.time = field[uint32](event, 20)
		w.press.button = int32(field[uint32](event, 52))
		w.press.rootX = field[float64](event, 64)
		w.press.rootY = field[float64](event, 72)
		w.press.hasPressed = true
		w.layerMove = nil
		if w.press.event != 0 {
			gdkEventFree(w.press.event)
		}
		w.press.event = gdkEventCopy(event)
		if edge := w.resizeEdge(event); edge >= 0 {
			// GdkEventButton: type 0; GDK_BUTTON_PRESS, not a double click.
			switch {
			case field[int32](event, 0) != 4 || w.press.button != 1:
			case w.layer:
				// As a move: the compositor resizes toplevels only.
				w.layerMove = &layerMove{edge: edge, start: w.layerBounds()}
			default:
				gtkWindowBeginResizeDrag(w.win, edge, w.press.button, int32(w.press.rootX), int32(w.press.rootY), w.press.time)
			}
			return true // the edges are not the page's
		}
		return false
	})
	cbMotion = purego.NewCallback(func(widget, event, data ptr) bool {
		w := b().window(data)
		if w == nil {
			return false
		}
		if w.layerMove != nil && w.moveLayer(event) {
			return true
		}
		edge := w.resizeEdge(event)
		w.showResizeCursor(edge)
		dragging := w.checkDragStart(event)
		return dragging || edge >= 0
	})
}
