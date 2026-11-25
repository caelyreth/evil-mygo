//go:build darwin && !ios

package darwin

import (
	"errors"
	"log"
	"math"

	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/platform"
)

const (
	styleTitled              = 1 << 0
	styleClosable            = 1 << 1
	styleMiniaturizable      = 1 << 2
	styleResizable           = 1 << 3
	styleFullScreen          = 1 << 14
	styleFullSizeContentView = 1 << 15
	styleNonactivatingPanel  = 1 << 7

	nsBackingStoreBuffered   = 2
	nsFloatingWindowLevel    = 3
	nsPopUpMenuWindowLevel   = 101
	nsNormalWindowLevel      = 0
	nsViewWidthHeightSizable = 2 | 16

	collectionCanJoinAllSpaces    = 1 << 0
	collectionTransient           = 1 << 3
	collectionIgnoresCycle        = 1 << 6
	collectionFullScreenPrimary   = 1 << 7
	collectionFullScreenAuxiliary = 1 << 8
	collectionFullScreenNone      = 1 << 9

	// NSApplicationPresentationOptions
	presentationAutoHideMenuBar = 1 << 2
	presentationFullScreen      = 1 << 10
	presentationAutoHideToolbar = 1 << 11
)

var (
	errInvalidImage = errors.New("mygo: invalid image data")
	errClosed       = errors.New("mygo: window has been closed")
)

type window struct {
	b        *Backend
	h        platform.WindowHandler
	opts     *platform.WindowOptions
	win      id
	view     id // the content view
	delegate id
	// effect is the material behind the content (Vibrancy): an
	// NSVisualEffectView, or an NSGlassEffectView when glass is.
	effect id
	glass  bool
	// keepActive is SetActiveAppearance's.
	keepActive bool
	// tint is the material's tint, which only NSGlassEffectView takes.
	tint *platform.Color
	// radius is the corners the content view is cut to (Flyout.Radius,
	// WindowOptions.CornerRadius), which the material follows.
	radius int
	// frameRadius is a titled window's own corner radius
	// (WindowOptions.CornerRadius), which its theme frame gives AppKit
	// (corners.go); 0 for the system's.
	frameRadius int
	// systemBacking tells that the content view of a rounded window has
	// the system's window color, which a layer cannot follow: it is
	// resolved again when the appearance changes (fillSystemBacking).
	systemBacking bool
	// glassClear tells that the glass made the window clear
	// (clearForGlass), an opaque one or the content view of a rounded one,
	// whose color waits in backing, retained, for the glass to go: 0 for
	// the system's (systemBacking).
	glassClear bool
	backing    id
	parent     *window
	// hiddenWithParent tells that a child window was ordered out as its
	// parent hid or miniaturized (hideChildren), at hiddenRank among its
	// siblings, front to back, to show again with it; orderPending that
	// orderChildrenLater posted the ordering of the window's children.
	hiddenWithParent bool
	hiddenRank       int
	orderPending     bool
	// hidMiniaturized tells that Hide ordered the window out while it
	// was miniaturized, which it then no longer shows as (IsVisible),
	// until it shows or deminiaturizes again.
	hidMiniaturized bool
	// onTop is what AlwaysOnTop asked for, which the window's level
	// follows, or its parent's when higher (applyLevel). attached tells
	// that a child window is an AppKit child window of its parent, which
	// is in full screen, and childrenAttached that the window attaches its
	// child windows, from entering full screen to leaving it
	// (attachChildren).
	onTop            bool
	attached         bool
	childrenAttached bool
	// flyout is how a flyout (WindowOptions.Flyout) was placed last, at
	// flyoutSize.
	flyout     platform.Flyout
	flyoutSize platform.Size
	// popover shows a flyout with the look of AppKit's popovers
	// (Flyout.Popover), in its window, win while it shows; popoverClosing
	// tells that the backend closes it, not the user; popoverHeld that
	// a window was modal to it as it showed (holdPopover); popoverShown
	// whether its surface was last told that some of it shows
	// (tellPopoverOcclusion).
	popover, popoverController  id
	popoverClosing, popoverHeld bool
	popoverShown                bool
	// ignoreMouse is SetIgnoreMouseEvents', and inputRegion
	// SetInputRegion's (input_region.go), nil for none, whose tracking
	// area is regionTracking; ignoring is whether the window lets the
	// mouse through, as either set it last.
	ignoreMouse, ignoring bool
	inputRegion           []platform.Rect
	regionTracking        id

	lastMouseDown id
	attention     int // the request of FlashFrame
	maximized     bool
	// auxiliary tells that SetVisibleOnAllWorkspaces made a window that
	// cannot go full screen an auxiliary of full-screen Spaces, to undo.
	auxiliary     bool
	programmatic  bool
	closed        bool
	sheet         bool
	trafficLights *platform.Point
	// surface shows the content MyGo draws.
	surface *surface
	// embeddedViews are views under the surface (embed.go), and host the
	// window whose surface an embedded view is under, in the clip view clip,
	// in pane, which shows it scaled and cut to its corners; mask is the
	// layer cutting pane to corners of different radii.
	embeddedViews []*window
	host          *window
	embeddedView  id
	clip          id
	pane          id
	mask          id
	// accessParent is the element of the content showing an embedded view.
	accessParent id
}

func (b *Backend) NewWindow(o *platform.WindowOptions, h platform.WindowHandler) (platform.Window, error) {
	w := &window{b: b, h: h, opts: o, trafficLights: o.TrafficLightPosition}
	if p, ok := o.Parent.(*window); ok && p != nil {
		w.parent = p
	}
	withPool(w.create)
	return w, nil
}

func (w *window) create() {
	o := w.opts

	if o.Flyout != nil {
		w.createFlyout()
		return
	}
	style := uint(styleTitled)
	if o.Closable {
		style |= styleClosable
	}
	if o.Minimizable {
		style |= styleMiniaturizable
	}
	if o.Resizable {
		style |= styleResizable
	}
	hiddenTitleBar := o.Frameless || o.TitleBarStyle == "hidden" || o.TitleBarStyle == "hiddenInset"
	if hiddenTitleBar {
		style |= styleFullSizeContentView
	}

	size := NSRect{Size: NSSize{float64(o.Width), float64(o.Height)}}
	content := size
	if !o.UseContentSize {
		content = msgRectForRect(class("NSWindow"), sel("contentRectForFrameRect:styleMask:"), size, style)
		content.Origin = NSPoint{}
	}
	cls := "MyGoWindow"
	if o.Panel {
		// The nonactivating style must be there from the start: the window
		// server only learns that the window never activates its app from
		// the style it is created with.
		cls = "MyGoPanel"
		style |= styleNonactivatingPanel
	}
	w.win = msgInitWindow(send(class(cls), "alloc"), sel("initWithContentRect:styleMask:backing:defer:"),
		content, style, nsBackingStoreBuffered, false)
	send(w.win, "setReleasedWhenClosed:", 0)
	w.delegate = alloc("MyGoWindowDelegate")
	send(w.win, "setDelegate:", uintptr(w.delegate))
	send(w.win, "setTitle:", uintptr(nsString(o.Title)))
	send(w.win, "setTabbingMode:", 2) // NSWindowTabbingModeDisallowed

	if hiddenTitleBar {
		send(w.win, "setTitlebarAppearsTransparent:", 1)
		send(w.win, "setTitleVisibility:", 1) // NSWindowTitleHidden
	}
	if o.Frameless || hiddenTitleBar && o.HideWindowButtons {
		for i := uintptr(0); i < 3; i++ {
			send(send(w.win, "standardWindowButton:", i), "setHidden:", 1)
		}
	}
	if o.TitleBarStyle == "hiddenInset" && !o.Frameless {
		tb := send(send(class("NSToolbar"), "alloc"), "initWithIdentifier:", uintptr(nsString("mygo.titlebar")))
		send(tb, "setShowsBaselineSeparator:", 0)
		send(w.win, "setToolbar:", uintptr(tb))
		release(tb)
	}
	if o.MinSize.Width > 0 || o.MinSize.Height > 0 {
		w.SetMinimumSize(o.MinSize)
	}
	if o.MaxSize.Width > 0 || o.MaxSize.Height > 0 {
		w.SetMaximumSize(o.MaxSize)
	}
	send(w.win, "setMovable:", boolArg(o.Movable))
	if !o.Maximizable {
		w.SetMaximizable(false)
	}
	behavior := uint(collectionFullScreenPrimary)
	if !o.Fullscreenable {
		behavior = collectionFullScreenNone
	}
	if o.Panel {
		// On the Space the user is on, full-screen ones included, as they
		// switch, and out of Mission Control and the window cycle.
		behavior = collectionCanJoinAllSpaces | collectionFullScreenAuxiliary | collectionIgnoresCycle | collectionTransient
		// A panel stays as the app is deactivated: the app hides it.
		send(w.win, "setHidesOnDeactivate:", 0)
		// NSPanel becomes key only when a view needs it (the default). A
		// hidden page has no first responder yet: Show would order the
		// panel front without the keyboard while another app is active.
		send(w.win, "setBecomesKeyOnlyIfNeeded:", 0)
	} else if w.modalToFlyout() {
		// Where its flyout is, over another app in full screen too.
		behavior = collectionFullScreenAuxiliary | collectionTransient
	}
	send(w.win, "setCollectionBehavior:", uintptr(behavior))
	// A child window takes its parent's level when that is higher.
	w.onTop = o.AlwaysOnTop
	w.applyLevel()
	radius := 0
	if o.Frameless {
		radius = o.CornerRadius
	} else {
		w.frameRadius = max(o.CornerRadius, 0) // AppKit draws them (corners.go)
	}
	w.fillWindow(content, radius)

	if o.Center {
		send(w.win, "center")
	} else {
		frame := msgRect(w.win, sel("frame"))
		r := rectFromMac(frame)
		r.X, r.Y = o.X, o.Y
		msgSetRectBool(w.win, sel("setFrame:display:"), rectToMac(r), false)
	}
	if w.trafficLights != nil {
		w.layoutTrafficLights()
		// The window's appearance changes with the application's, or when
		// it is given its own; either lays the title bar out again.
		send(w.win, "addObserver:forKeyPath:options:context:", uintptr(w.delegate), uintptr(nsString("effectiveAppearance")), 0, 0)
	}
	w.register()
	if w.frameRadius > 0 { // registered: the theme frame finds the window's radius now
		w.invalidateCorners()
	}

	if o.FullScreen {
		send(w.win, "toggleFullScreen:", 0)
	} else if o.Maximized {
		send(w.win, "zoom:", 0)
	}
}

// nsWindowAnimationBehaviorNone is NSWindowAnimationBehaviorNone.
const nsWindowAnimationBehaviorNone = 2

// createFlyout creates the panel of a flyout (WindowOptions.Flyout):
// borderless and nonactivating, so that it never activates the app,
// and the key window only when it is focusable, while its parent stays
// the main window. It is a child window of its parent while it shows.
func (w *window) createFlyout() {
	o := w.opts
	content := NSRect{Size: NSSize{float64(o.Width), float64(o.Height)}}
	w.flyout = *o.Flyout
	w.flyoutSize = platform.Size{Width: o.Width, Height: o.Height}
	if w.flyout.Popover {
		w.createPopover(content)
		return
	}
	w.win = msgInitWindow(send(class("MyGoFlyout"), "alloc"), sel("initWithContentRect:styleMask:backing:defer:"),
		content, styleNonactivatingPanel, nsBackingStoreBuffered, false)
	send(w.win, "setReleasedWhenClosed:", 0)
	w.delegate = alloc("MyGoWindowDelegate")
	send(w.win, "setDelegate:", uintptr(w.delegate))
	send(w.win, "setTitle:", uintptr(nsString(o.Title)))
	// NSPanel hides when the app is deactivated: a flyout stays with its
	// parent instead, and one that takes the keyboard closes.
	send(w.win, "setHidesOnDeactivate:", 0)
	send(w.win, "setCollectionBehavior:", collectionTransient|collectionIgnoresCycle|collectionFullScreenAuxiliary)
	// AppKit grows a panel in as it orders it in, as an alert, its content
	// oversized for a moment: a flyout shows at once, as a menu does.
	send(w.win, "setAnimationBehavior:", nsWindowAnimationBehaviorNone)
	if w.flyout.Tray != nil {
		// Over the windows of other apps, as the menus of the menu bar.
		send(w.win, "setLevel:", nsPopUpMenuWindowLevel)
	}
	w.fillWindow(content, w.flyout.Radius)
	w.register()
	w.PlaceFlyout(w.flyout, w.flyoutSize)
}

// roundContent cuts the content view, and the material in it, to corners
// of radius, a flyout's (Flyout.Radius) or a frameless window's
// (WindowOptions.CornerRadius). The window is made clear, its color moved
// to the content view, which is cut to them, so that nothing fills the
// corners, and its shadow, made from what it draws, follows them.
func (w *window) roundContent(radius int) {
	if radius <= 0 {
		return
	}
	w.radius = radius
	send(w.view, "setWantsLayer:", 1)
	layer := send(w.view, "layer")
	msgSetFloat(layer, sel("setCornerRadius:"), float64(radius))
	send(layer, "setCornerCurve:", uintptr(nsString("continuous")))
	send(layer, "setMasksToBounds:", 1)
	if sendBool(w.win, "isOpaque") {
		if w.opts.BackgroundColor == nil {
			w.systemBacking = true
			w.fillSystemBacking()
		} else {
			send(layer, "setBackgroundColor:", uintptr(send(send(w.win, "backgroundColor"), "CGColor")))
		}
		send(w.win, "setOpaque:", 0)
		send(w.win, "setBackgroundColor:", uintptr(send(class("NSColor"), "clearColor")))
	}
	w.roundEffect()
	send(w.win, "invalidateShadow")
}

// fillSystemBacking gives the content view of a rounded window without a
// BackgroundColor the window color of the system, as the app's appearance
// draws it. A layer's color is fixed once set, where the window's followed
// the appearance: refreshSystemBackings sets it again when that changes.
func (w *window) fillSystemBacking() {
	withPool(func() {
		appearance := class("NSAppearance")
		prev := send(appearance, "currentAppearance")
		send(appearance, "setCurrentAppearance:", uintptr(send(w.b.app, "effectiveAppearance")))
		c := send(send(class("NSColor"), "windowBackgroundColor"), "CGColor")
		send(send(w.view, "layer"), "setBackgroundColor:", uintptr(c))
		send(appearance, "setCurrentAppearance:", uintptr(prev))
	})
}

// refreshSystemBackings resolves the window color of rounded windows
// anew, once the appearance or the system's colors changed.
func (b *Backend) refreshSystemBackings() {
	for _, w := range b.byNSWindow {
		if w.popover != 0 {
			send(w.popover, "setAppearance:", uintptr(send(b.app, "effectiveAppearance")))
		}
		if w.systemBacking && !w.glassClear {
			w.fillSystemBacking()
		}
	}
}

// roundEffect cuts the material to the content view's corners. The glass
// draws its rim along its own corners, which it rounds itself.
func (w *window) roundEffect() {
	if w.effect == 0 || w.radius <= 0 {
		return
	}
	if w.glass {
		msgSetFloat(w.effect, sel("setCornerRadius:"), float64(w.radius))
		return
	}
	send(w.effect, "setWantsLayer:", 1)
	el := send(w.effect, "layer")
	msgSetFloat(el, sel("setCornerRadius:"), float64(w.radius))
	send(el, "setCornerCurve:", uintptr(nsString("continuous")))
	send(el, "setMasksToBounds:", 1)
}

// fillWindow gives a new window its look, its corners of radius
// (roundContent) and its web view or surface.
func (w *window) fillWindow(content NSRect, radius int) {
	o := w.opts
	send(w.win, "setHasShadow:", boolArg(o.HasShadow))
	if o.Opacity < 1 {
		msgSetFloat(w.win, sel("setAlphaValue:"), o.Opacity)
	}
	if o.Transparent {
		send(w.win, "setOpaque:", 0)
		send(w.win, "setBackgroundColor:", uintptr(send(class("NSColor"), "clearColor")))
	} else if o.BackgroundColor != nil {
		send(w.win, "setBackgroundColor:", uintptr(nsColor(*o.BackgroundColor)))
	}

	w.createContent(content)
	if w.surface != nil {
		send(w.win, "makeFirstResponder:", uintptr(w.surface.view))
	}
	send(w.win, "setContentView:", uintptr(w.view))
	// Before the material: the glass clears the color roundContent moves.
	w.roundContent(radius)
	if o.Vibrancy != "" {
		w.SetVibrancy(o.Vibrancy)
	}
	if o.VibrancyTint != nil {
		w.SetVibrancyTint(o.VibrancyTint)
	}
}

// createContent creates the surface in w.view.
func (w *window) createContent(content NSRect) {
	w.createSurface(content)
	w.view = msgInitRect(send(class("NSView"), "alloc"), sel("initWithFrame:"), NSRect{Size: content.Size})
	send(w.view, "addSubview:", uintptr(w.surface.view))
}

// register lets the delegate and the window find w.
func (w *window) register() {
	b := w.b
	b.byDelegate[w.delegate] = w
	if w.win != 0 {
		b.byNSWindow[w.win] = w
	}
}

// PlaceFlyout places a flyout next to its anchor, in the parent's content
// area, as Flyout.Place resolves it in the visible frame of a screen.
func (w *window) PlaceFlyout(f platform.Flyout, size platform.Size) {
	if w.opts.Flyout == nil || w.closed {
		return
	}
	w.flyout, w.flyoutSize = f, size
	if w.popover != 0 {
		w.placePopover()
		return
	}
	displays := screen{}.Displays()
	anchor := f.Anchor
	if t, ok := f.Tray.(*tray); ok {
		if anchor = t.Bounds(); anchor.Width == 0 || anchor.Height == 0 {
			r := platform.Centered(size, platform.PrimaryWorkArea(displays))
			msgSetRectBool(w.win, sel("setFrame:display:"), rectToMac(r), true)
			return
		}
	} else if p := w.parent; p != nil && !p.closed {
		c := p.ContentBounds()
		anchor.X += c.X
		anchor.Y += c.Y
	}
	r, _, _ := f.Place(anchor, size, platform.WorkAreaFor(anchor, displays))
	msgSetRectBool(w.win, sel("setFrame:display:"), rectToMac(r), true)
}

func nsColor(c platform.Color) id {
	return msgColor(class("NSColor"), sel("colorWithSRGBRed:green:blue:alpha:"),
		float64(c.R)/255, float64(c.G)/255, float64(c.B)/255, float64(c.A)/255)
}

// cleanup releases native resources; called from windowWillClose:.
func (w *window) cleanup() {
	if w.closed {
		return
	}
	popover := w.popover != 0
	if !popover {
		w.orderOutClosing()
	}
	w.closeEmbeddedViews()
	w.closed = true
	w.releaseChildren()
	b := w.b
	if w.trafficLights != nil {
		send(w.win, "removeObserver:forKeyPath:", uintptr(w.delegate), uintptr(nsString("effectiveAppearance")))
	}
	if popover {
		w.cleanupPopover()
	}
	delete(b.byDelegate, w.delegate)
	delete(b.byNSWindow, w.win)
	if inputRegionClosed != nil {
		inputRegionClosed(w)
	}
	w.forgetDrag()
	if w.surface != nil {
		w.surface.destroy()
	}
	release(w.effect)
	release(w.backing)
	release(w.view)
	// AppKit is still closing the window; let the pool release it.
	autorelease(w.delegate)
	if !popover {
		autorelease(w.win)
	}
}

// orderOutClosing orders out a window that closes, before cleanup takes
// its content down: AppKit sends windowWillClose: before it orders the
// window out, with the animation that fades it, which then faded a
// window with nothing in it, and a light one flashed white (#1931).
// Ordered out here, whether Close or the user closes it, the window
// fades with its last frame, and AppKit's own ordering out finds nothing
// left to do. The delegate goes first, so the window tells nothing of
// losing the keyboard as it goes, as before; a window modal to a flyout
// gives the keyboard back to it first, as Hide does. A window that does
// not show, hidden or minimized, is left to AppKit.
func (w *window) orderOutClosing() {
	send(w.win, "setDelegate:", 0)
	if w.modalToFlyout() {
		w.leaveFlyout()
	} else if w.parent != nil && !w.parent.closed {
		send(w.parent.win, "removeChildWindow:", uintptr(w.win))
	}
	if sendBool(w.win, "isVisible") {
		send(w.win, "orderOut:", 0)
	}
}

func (w *window) Handle() uintptr { return uintptr(w.win) }

func (w *window) SetTitle(title string) {
	withPool(func() { send(w.win, "setTitle:", uintptr(nsString(title))) })
	w.layoutTrafficLights()
}

// SetRepresentedFile shows the file's proxy icon in the title bar, which
// the user drags, and whose ⌘-click menu lists its folders.
func (w *window) SetRepresentedFile(path string) {
	withPool(func() {
		var url id
		if path != "" {
			url = fileURL(path)
		}
		send(w.win, "setRepresentedURL:", uintptr(url))
	})
	// AppKit lays the title bar's buttons out again.
	w.layoutTrafficLights()
}

// SetDocumentEdited shows the dot in the close button, and dims the proxy
// icon.
func (w *window) SetDocumentEdited(edited bool) {
	send(w.win, "setDocumentEdited:", boolArg(edited))
	w.layoutTrafficLights()
}

func (w *window) Title() string {
	var s string
	withPool(func() { s = goString(send(w.win, "title")) })
	return s
}

func (w *window) SetBounds(r platform.Rect) {
	msgSetRectBool(w.win, sel("setFrame:display:"), rectToMac(r), true)
}

func (w *window) Bounds() platform.Rect { return rectFromMac(msgRect(w.win, sel("frame"))) }

func (w *window) SetContentBounds(r platform.Rect) {
	frame := msgRectToRect(w.win, sel("frameRectForContentRect:"), rectToMac(r))
	msgSetRectBool(w.win, sel("setFrame:display:"), frame, true)
}

func (w *window) ContentBounds() platform.Rect {
	frame := msgRect(w.win, sel("frame"))
	return rectFromMac(msgRectToRect(w.win, sel("contentRectForFrameRect:"), frame))
}

func (w *window) SetMinimumSize(s platform.Size) {
	msgSetSize(w.win, sel("setMinSize:"), NSSize{float64(s.Width), float64(s.Height)})
}

func (w *window) SetMaximumSize(s platform.Size) {
	max := NSSize{math.MaxFloat32, math.MaxFloat32}
	if s.Width > 0 {
		max.Width = float64(s.Width)
	}
	if s.Height > 0 {
		max.Height = float64(s.Height)
	}
	msgSetSize(w.win, sel("setMaxSize:"), max)
}

func (w *window) setStyle(bit uint, on bool) {
	mask := uint(send(w.win, "styleMask"))
	if on {
		mask |= bit
	} else {
		mask &^= bit
	}
	send(w.win, "setStyleMask:", uintptr(mask))
}

func (w *window) hasStyle(bit uint) bool { return uint(send(w.win, "styleMask"))&bit != 0 }

func (w *window) SetResizable(v bool)   { w.setStyle(styleResizable, v) }
func (w *window) IsResizable() bool     { return w.hasStyle(styleResizable) }
func (w *window) SetMovable(v bool)     { send(w.win, "setMovable:", boolArg(v)) }
func (w *window) IsMovable() bool       { return sendBool(w.win, "isMovable") }
func (w *window) SetMinimizable(v bool) { w.setStyle(styleMiniaturizable, v) }
func (w *window) IsMinimizable() bool   { return w.hasStyle(styleMiniaturizable) }
func (w *window) SetClosable(v bool)    { w.setStyle(styleClosable, v) }
func (w *window) IsClosable() bool      { return w.hasStyle(styleClosable) }

func (w *window) SetMaximizable(v bool) {
	send(send(w.win, "standardWindowButton:", 2), "setEnabled:", boolArg(v))
}

func (w *window) IsMaximizable() bool {
	return sendBool(send(w.win, "standardWindowButton:", 2), "isEnabled")
}

func (w *window) SetAlwaysOnTop(v bool) {
	w.onTop = v
	if w.host != nil || w.opts.Flyout != nil {
		level := uintptr(nsNormalWindowLevel)
		if v {
			level = nsFloatingWindowLevel
		}
		send(w.win, "setLevel:", level)
		return
	}
	// Its child windows follow it to its level (applyLevel).
	w.applyLevel()
	if w.onScreen() {
		w.orderChildrenLater()
	}
}

func (w *window) IsAlwaysOnTop() bool {
	if w.isChild() {
		return w.onTop // its level is its parent's when that is higher
	}
	return sendInt(w.win, "level") != nsNormalWindowLevel
}

func (w *window) Show() {
	if w.popover != 0 {
		w.showPopover()
		return
	}
	if w.opts.Flyout != nil {
		w.showFlyout(w.flyout.Focusable)
		return
	}
	if w.modalToFlyout() {
		w.showOverFlyout(true)
		return
	}
	if w.parent != nil && w.opts.Modal && !w.sheet {
		w.sheet = true
		send(w.parent.win, "beginSheet:completionHandler:", uintptr(w.win), 0)
		return
	}
	w.showAboveParent(true)
	if w.opts.Panel {
		w.takeKeyWithoutActivating()
		return
	}
	send(w.b.app, "activateIgnoringOtherApps:", 1)
}

// A window modal to a flyout (modalToFlyout) is no sheet of it, which
// hangs from its top edge, no wider than a flyout is: it shows where the
// core put it, over the flyout, attached as its AppKit child window at
// its level, as a sheet is, which keeps it above the flyout. The flyout
// takes no input while it shows (watchModalInput), and keeps open
// (modalChild): it neither closes as it resigns key, nor on a click
// outside the app, and its popover is held open (holdPopover).

// modalToFlyout tells that w is a window modal to a flyout.
func (w *window) modalToFlyout() bool {
	return w.opts.Modal && w.parent != nil && w.parent.opts.Flyout != nil
}

// modalChild returns the window modal to w that shows, or nil.
func (w *window) modalChild() *window {
	for _, c := range w.b.byNSWindow {
		if c.parent == w && c.modalToFlyout() && !c.closed && sendBool(c.win, "isVisible") {
			return c
		}
	}
	return nil
}

// showOverFlyout shows a window modal to a flyout above it, as the key
// window for Show (key).
func (w *window) showOverFlyout(key bool) {
	w.b.watchModalInput()
	w.applyLevel() // a popover's window is made as it shows
	p := w.parent
	p.holdPopover()
	if key {
		send(w.b.app, "activateIgnoringOtherApps:", 1)
		send(w.win, "makeKeyAndOrderFront:", 0)
	} else {
		send(w.win, "orderFrontRegardless")
	}
	if !p.closed && p.win != 0 && sendBool(p.win, "isVisible") && send(w.win, "parentWindow") == 0 {
		send(p.win, "addChildWindow:ordered:", uintptr(w.win), 1) // NSWindowAbove
	}
}

// leaveFlyout detaches a window modal to a flyout from it as it hides or
// closes, and gives the keyboard it had back to the flyout when that
// takes it, else to the flyout's parent.
func (w *window) leaveFlyout() {
	p := w.parent
	key := sendBool(w.win, "isKeyWindow")
	if p.closed || p.win == 0 {
		return
	}
	send(p.win, "removeChildWindow:", uintptr(w.win))
	if !key || !p.IsVisible() {
		return
	}
	if p.flyout.Focusable {
		send(p.win, "makeKeyWindow")
	} else if pp := p.parent; pp != nil && !pp.closed && sendBool(pp.win, "isVisible") {
		send(pp.win, "makeKeyWindow")
	}
}

// modalInput is the monitor of the app's events that eats the mouse's and
// the keyboard's in a flyout a window is modal to, as AppKit's in a
// window under its sheet: a press brings the modal window in front, with
// the keyboard. It is made once, as the first such window shows.
var modalInput id

func (b *Backend) watchModalInput() {
	if modalInput != 0 {
		return
	}
	// left, right and other mouse down, up and dragged, mouse moved, key
	// down and up, scroll wheel
	const mask = 1<<1 | 1<<3 | 1<<25 | 1<<2 | 1<<4 | 1<<26 | 1<<6 | 1<<7 | 1<<27 | 1<<5 | 1<<10 | 1<<11 | 1<<22
	blk := newBlock(func(_ objc.Block, ev id) id {
		f := b.byNSWindow[send(ev, "window")]
		if f == nil || f.closed || f.opts.Flyout == nil {
			return ev
		}
		m := f.modalChild()
		if m == nil {
			return ev
		}
		switch sendInt(ev, "type") {
		case 1, 3, 25: // NSEventTypeLeftMouseDown, RightMouseDown, OtherMouseDown
			send(b.app, "activateIgnoringOtherApps:", 1)
			send(m.win, "makeKeyAndOrderFront:", 0)
		}
		return 0
	})
	modalInput = retain(send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", mask, uintptr(blk)))
}

// showFlyout shows a flyout above its parent, as its child window, and
// gives it the keyboard when it takes it. Showing it does not activate
// the app: it shows over a parent the user is using.
func (w *window) showFlyout(key bool) {
	fade := w.flyout.FadeIn && !sendBool(w.win, "isVisible") &&
		!sendBool(workspace(), "accessibilityDisplayShouldReduceMotion")
	if fade {
		msgSetFloat(w.win, sel("setAlphaValue:"), 0)
		defer w.fadeIn()
	}
	if key {
		w.b.watchOutsideClicks()
		send(w.win, "makeKeyAndOrderFront:", 0)
	} else {
		send(w.win, "orderFront:", 0)
	}
	if p := w.parent; p != nil && !p.closed && send(w.win, "parentWindow") == 0 {
		send(p.win, "addChildWindow:ordered:", uintptr(w.win), 1) // NSWindowAbove
	}
}

// flyoutFade is how long a flyout that fades in (Flyout.FadeIn) takes.
const flyoutFade = 0.14

// fadeIn brings the window's alpha to 1 over flyoutFade, easing out, its
// material, shadow and content together.
func (w *window) fadeIn() {
	withPool(func() {
		ctx := class("NSAnimationContext")
		send(ctx, "beginGrouping")
		cur := send(ctx, "currentContext")
		msgSetFloat(cur, sel("setDuration:"), flyoutFade)
		send(cur, "setTimingFunction:", uintptr(send(class("CAMediaTimingFunction"), "functionWithName:", uintptr(nsString("easeOut")))))
		msgSetFloat(send(w.win, "animator"), sel("setAlphaValue:"), 1)
		send(ctx, "endGrouping")
	})
}

// hideFlyout hides a flyout. It stops being a child window, which AppKit
// would order in again with its parent, and gives the keyboard it had
// back to the parent.
func (w *window) hideFlyout() {
	key := sendBool(w.win, "isKeyWindow")
	if p := w.parent; p != nil && !p.closed {
		send(p.win, "removeChildWindow:", uintptr(w.win))
		if key && sendBool(p.win, "isVisible") {
			send(p.win, "makeKeyWindow")
		}
	}
	send(w.win, "orderOut:", 0)
}

// outsideClicks is the monitor of clicks outside the app, in other apps or
// the menu bar, which close focusable flyouts as AppKit's transient
// popovers close: their panels may take the keyboard without the app
// being activated, and a click in the menu bar activates no other app, so
// they would not resign the key window. It is made once, as the first
// focusable flyout shows.
var outsideClicks id

func (b *Backend) watchOutsideClicks() {
	if outsideClicks != 0 {
		return
	}
	const mask = 1<<1 | 1<<3 | 1<<25 // left, right and other mouse down
	blk := newBlock(func(_ objc.Block, ev id) {
		var open []*window
		for _, w := range b.byNSWindow {
			if w.opts.Flyout != nil && w.popover == 0 && w.flyout.Focusable && !w.closed && sendBool(w.win, "isVisible") && w.modalChild() == nil {
				// A press on its tray icon is left to the icon's click,
				// which toggles it, rather than closing it for the click
				// to open it again.
				if t, ok := w.flyout.Tray.(*tray); ok && t.pressed() {
					continue
				}
				open = append(open, w)
			}
		}
		for _, w := range open {
			if !w.closed && w.h.ShouldClose() {
				w.Close()
			}
		}
		b.closeHeldPopovers(0)
	})
	outsideClicks = retain(send(class("NSEvent"), "addGlobalMonitorForEventsMatchingMask:handler:", mask, uintptr(blk)))
	b.watchAnchorPresses()
}

// anchorPresses is the monitor of presses in the app that eats a press on
// the anchor of a focusable flyout, which only closes the flyout, as
// AppKit's transient popovers eat a press on their positioning rect: the
// press would make the parent key, which closes the flyout before the
// release clicks what opened it, which would open it again. The press's
// drags and release are eaten too. Presses elsewhere go on to what is
// under the pointer. It is made once, with outsideClicks.
var anchorPresses id

// eatingPress is the type of the release that ends a press anchorPresses
// ate, or 0. Main thread only.
var eatingPress int

func (b *Backend) watchAnchorPresses() {
	// left, right and other mouse down, up and dragged
	const mask = 1<<1 | 1<<3 | 1<<25 | 1<<2 | 1<<4 | 1<<26 | 1<<6 | 1<<7 | 1<<27
	blk := newBlock(func(_ objc.Block, ev id) id {
		switch typ := sendInt(ev, "type"); typ {
		case 2, 4, 26: // NSEventTypeLeftMouseUp, RightMouseUp, OtherMouseUp
			if typ == eatingPress {
				eatingPress = 0
				return 0
			}
		case 6, 7, 27: // NSEventTypeLeftMouseDragged, RightMouseDragged, OtherMouseDragged
			if eatingPress != 0 {
				return 0
			}
		default: // a press, which ends any other
			eatingPress = 0
			b.closeHeldPopovers(send(ev, "window"))
			if w := b.flyoutAnchoredAt(ev); w != nil {
				eatingPress = typ + 1
				if w.h.ShouldClose() {
					w.Close()
				}
				return 0
			}
		}
		return ev
	})
	anchorPresses = retain(send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", mask, uintptr(blk)))
}

// flyoutAnchoredAt returns the focusable flyout that shows in a panel
// whose anchor is under the press ev in its parent, or nil.
func (b *Backend) flyoutAnchoredAt(ev id) *window {
	p := b.byNSWindow[send(ev, "window")]
	if p == nil || p.closed {
		return nil
	}
	frame := msgRect(p.win, sel("frame"))
	c := msgRectToRect(p.win, sel("contentRectForFrameRect:"), frame)
	loc := msgPoint(ev, sel("locationInWindow"))
	x := loc.X - (c.Origin.X - frame.Origin.X)
	y := c.Origin.Y - frame.Origin.Y + c.Size.Height - loc.Y
	for _, w := range b.byNSWindow {
		if w.parent != p || w.opts.Flyout == nil || w.popover != 0 || !w.flyout.Focusable || w.closed || !sendBool(w.win, "isVisible") {
			continue
		}
		if a := w.flyout.Anchor; x >= float64(a.X) && y >= float64(a.Y) && x < float64(a.X+a.Width) && y < float64(a.Y+a.Height) {
			return w
		}
	}
	return nil
}

// dismissFlyout closes a focusable flyout that lost the keyboard, as the
// user closing it, unless it went to a flyout or a window of its own, or
// a window is modal to it.
func (w *window) dismissFlyout() {
	if w.closed || !w.flyout.Focusable || !sendBool(w.win, "isVisible") || w.modalChild() != nil {
		return
	}
	for k := w.b.byNSWindow[send(w.b.app, "keyWindow")]; k != nil; k = k.parent {
		if k == w {
			return
		}
	}
	if w.h.ShouldClose() {
		w.Close()
	}
}

func (w *window) ShowInactive() {
	if w.popover != 0 {
		w.showPopover()
		return
	}
	if w.opts.Flyout != nil {
		w.showFlyout(false)
		return
	}
	if w.modalToFlyout() {
		w.showOverFlyout(false)
		return
	}
	w.showAboveParent(false)
}

func (w *window) Hide() {
	if w.popover != 0 {
		w.hidePopover()
		return
	}
	if w.opts.Flyout != nil {
		w.hideFlyout()
		return
	}
	if w.sheet {
		w.sheet = false
		send(w.parent.win, "endSheet:", uintptr(w.win))
		return
	}
	if w.modalToFlyout() {
		w.leaveFlyout()
		send(w.win, "orderOut:", 0)
		return
	}
	w.hiddenWithParent = false
	w.hidMiniaturized = w.IsMinimized()
	// Attached, AppKit would show it again with its parent.
	w.detach()
	send(w.win, "orderOut:", 0)
	w.hideChildren()
}

// IsVisible reports whether Show (or the options) showed the window and
// Hide did not hide it since. AppKit's isVisible is false for a window
// in the Dock, which counts as shown here, as IsWindowVisible on Windows
// and gtk_widget_get_visible on Linux count a minimized window (#1542).
func (w *window) IsVisible() bool {
	if w.popover != 0 {
		return sendBool(w.popover, "isShown")
	}
	return sendBool(w.win, "isVisible") || !w.hidMiniaturized && w.IsMinimized()
}

// takeKeyWithoutActivating makes a panel the key window without activating
// the app. Show and Focus already ordered it front; NSApp may still be
// resigning active (Finder can be frontmost first, see TestGlass), which
// would take the key back. The next turn takes it again if that happens.
func (w *window) takeKeyWithoutActivating() {
	if !sendBool(w.win, "isKeyWindow") {
		send(w.win, "makeKeyWindow")
	}
	w.b.post(func() {
		if w.closed || !sendBool(w.win, "isVisible") || sendBool(w.win, "isKeyWindow") {
			return
		}
		send(w.win, "makeKeyWindow")
	})
}

func (w *window) Focus() {
	if w.host != nil {
		if !w.closed {
			send(w.win, "makeFirstResponder:", uintptr(w.embeddedView))
		}
		return
	}
	if w.opts.Flyout != nil {
		w.Show() // the key window only when it takes the keyboard
		return
	}
	w.hiddenWithParent, w.hidMiniaturized = false, false
	send(w.win, "makeKeyAndOrderFront:", 0)
	if w.opts.Panel {
		w.takeKeyWithoutActivating()
		return
	}
	send(w.b.app, "activateIgnoringOtherApps:", 1)
}

func (w *window) Blur() { send(w.win, "orderBack:", 0) }
func (w *window) IsFocused() bool {
	return sendBool(w.win, "isKeyWindow")
}

// CornerRadius is the radius of the window's corners: a rounded frameless
// window's (roundContent), or the one the system draws a titled window
// with. AppKit tells that one only through NSWindow's _cornerRadius, read
// when the window answers it; it is 26 on macOS 26 with a toolbar, 16
// without one, smaller before. A window in full screen has none.
func (w *window) CornerRadius() float64 {
	switch {
	case w.host != nil || w.IsFullScreen():
		return 0
	case w.radius > 0:
		return float64(w.radius)
	case w.frameRadius > 0: // a titled window's own (corners.go)
		return float64(w.frameRadius)
	case !w.hasStyle(styleTitled) || !respondsTo(w.win, "_cornerRadius"):
		return 0
	}
	return max(msgFloat(w.win, sel("_cornerRadius")), 0)
}

func (w *window) Minimize()         { send(w.win, "miniaturize:", 0) }
func (w *window) IsMinimized() bool { return sendBool(w.win, "isMiniaturized") }

func (w *window) Maximize() {
	if !w.IsMaximized() {
		send(w.win, "zoom:", 0)
	}
}

func (w *window) Unmaximize() {
	if w.IsMaximized() {
		send(w.win, "zoom:", 0)
	}
}

func (w *window) IsMaximized() bool { return sendBool(w.win, "isZoomed") && !w.IsFullScreen() }

func (w *window) Restore() {
	if w.IsMinimized() {
		send(w.win, "deminiaturize:", 0)
	}
}

func (w *window) SetFullScreen(v bool) {
	if v != w.IsFullScreen() {
		send(w.win, "toggleFullScreen:", 0)
	}
}

func (w *window) IsFullScreen() bool { return w.hasStyle(styleFullScreen) }
func (w *window) Center()            { send(w.win, "center") }

func (w *window) SetBackgroundColor(c platform.Color) {
	withPool(func() {
		switch {
		case w.glassClear:
			// The glass shows what is behind the window, the color once it goes.
			w.systemBacking = false
			release(w.backing)
			w.backing = retain(nsColor(c))
		case w.radius > 0:
			// The window stays clear, its corners cut off (roundContent).
			w.systemBacking = false
			send(send(w.view, "layer"), "setBackgroundColor:", uintptr(send(nsColor(c), "CGColor")))
		default:
			send(w.win, "setBackgroundColor:", uintptr(nsColor(c)))
		}
	})
}

func (w *window) SetOpacity(v float64) { msgSetFloat(w.win, sel("setAlphaValue:"), v) }

func (w *window) SetProgressBar(state string, value float64) { w.b.setDockProgress(state, value) }

func (w *window) FlashFrame(flash bool) {
	if w.attention != 0 {
		send(w.b.app, "cancelUserAttentionRequest:", uintptr(w.attention))
		w.attention = 0
	}
	if flash {
		w.attention = sendInt(w.b.app, "requestUserAttention:", 10) // NSInformationalRequest
	}
}

// SetSkipTaskbar does nothing: the Dock shows applications, not windows.
func (w *window) SetSkipTaskbar(bool) {}

func (w *window) SetVisibleOnAllWorkspaces(v bool) {
	const canJoinAllSpaces = 1 << 0
	behavior := uint(sendInt(w.win, "collectionBehavior"))
	if v {
		behavior |= canJoinAllSpaces
		// FullScreenNone (DisableFullScreen) also keeps a window out of
		// other apps' full-screen Spaces. As an auxiliary, the window shows
		// there too (an accessory app's: macOS shows no regular app's
		// windows there), and still cannot go full screen itself.
		if behavior&collectionFullScreenNone != 0 {
			behavior = behavior&^collectionFullScreenNone | collectionFullScreenAuxiliary
			w.auxiliary = true
		}
	} else {
		behavior &^= canJoinAllSpaces
		if w.auxiliary {
			behavior = behavior&^collectionFullScreenAuxiliary | collectionFullScreenNone
			w.auxiliary = false
		}
	}
	send(w.win, "setCollectionBehavior:", uintptr(behavior))
}

// SetIcon does nothing: macOS windows show no icon of their own.
func (w *window) SetIcon([]byte) error { return nil }
func (w *window) Opacity() float64     { return msgFloat(w.win, sel("alphaValue")) }
func (w *window) SetHasShadow(v bool)  { send(w.win, "setHasShadow:", boolArg(v)) }
func (w *window) HasShadow() bool      { return sendBool(w.win, "hasShadow") }

func (w *window) SetContentProtection(v bool) {
	sharing := uintptr(1) // NSWindowSharingReadOnly
	if v {
		sharing = 0 // NSWindowSharingNone
	}
	send(w.win, "setSharingType:", sharing)
}

// glassStyles maps the Liquid Glass vibrancies to NSGlassEffectViewStyle,
// and to the NSVisualEffectMaterial closest to each before macOS 26, which
// has no NSGlassEffectView.
var glassStyles = map[string]struct{ style, material uintptr }{
	"glass":       {0, 6},  // regular; popover
	"glass-clear": {1, 13}, // clear; HUD
}

// vibrancyMaterials maps mygo.Vibrancy values to NSVisualEffectMaterial.
var vibrancyMaterials = map[string]uintptr{
	"titlebar": 3, "selection": 4, "menu": 5, "popover": 6, "sidebar": 7, "header": 10,
	"sheet": 11, "window": 12, "hud": 13, "fullscreen-ui": 15, "tooltip": 17, "content": 18,
	"under-window": 21, "under-page": 22,
	// Windows 11 backdrops.
	"mica": 21, "tabbed": 21, "acrylic": 13,
}

func (w *window) SetVibrancy(material string) {
	withPool(func() {
		m, ok := vibrancyMaterials[material]
		g, isGlass := glassStyles[material]
		glass := isGlass && hasClass("NSGlassEffectView")
		if isGlass && !glass {
			m, ok = g.material, true
		}
		if !ok && !glass {
			if material != "" {
				log.Printf("mygo: unknown vibrancy %q", material)
			}
			w.removeEffect()
			return
		}
		if w.effect != 0 && w.glass != glass {
			w.removeEffect()
		}
		if w.effect == 0 {
			cls := "NSVisualEffectView"
			if glass {
				cls = "NSGlassEffectView"
			}
			w.effect = msgInitRect(send(class(cls), "alloc"), sel("initWithFrame:"), msgRect(w.view, sel("bounds")))
			w.glass = glass
			if !glass {
				send(w.effect, "setBlendingMode:", 0) // NSVisualEffectBlendingModeBehindWindow
				send(w.effect, "setState:", w.effectState())
			}
			send(w.effect, "setAutoresizingMask:", nsViewWidthHeightSizable)
			send(w.view, "addSubview:positioned:relativeTo:", uintptr(w.effect), ^uintptr(0), 0) // NSWindowBelow
			w.roundEffect()
			if glass {
				w.clearForGlass()
			}
		}
		if glass {
			send(w.effect, "setStyle:", g.style)
			w.applyTint()
		} else {
			send(w.effect, "setMaterial:", m)
		}
	})
}

// SetActiveAppearance has the window drawn as the main window of the app
// in front, whichever is, through AppKit's own, private
// _setForceMainAppearance:. Without it, a window that is not draws its glass
// frosted, the clear style too, as a panel floating over another app always
// is. NSVisualEffectView has a public setting for it (its state, which
// SetVibrancy sets); NSGlassEffectView has none.
func (w *window) SetActiveAppearance(v bool) {
	w.keepActive = v
	if respondsTo(w.win, "_setForceMainAppearance:") {
		send(w.win, "_setForceMainAppearance:", boolArg(v))
	}
	if w.effect != 0 && !w.glass {
		send(w.effect, "setState:", w.effectState())
	}
}

// effectState returns the NSVisualEffectState of the window's material: a
// window's follows its active state, as AppKit's sidebars do, and fades to
// an opaque gray while the window is not the one the user's input goes to.
// Panels and flyouts float over whatever app is in front, never active as
// AppKit tells it, and keep theirs active, as SetActiveAppearance keeps a
// window's.
func (w *window) effectState() uintptr {
	if w.keepActive || w.opts.Panel || w.opts.Flyout != nil {
		return 1 // NSVisualEffectStateActive
	}
	return 0 // NSVisualEffectStateFollowsWindowActiveState
}

// SetWindowButtonsTheme does nothing: the traffic lights look the same in
// both appearances, and an appearance of the window's own would change its
// page's too.
func (w *window) SetWindowButtonsTheme(string) {}

// SetVibrancyTint tints the glass, nil to remove the tint. Other
// materials take none.
func (w *window) SetVibrancyTint(c *platform.Color) {
	w.tint = c
	if w.effect != 0 && w.glass {
		withPool(w.applyTint)
	}
}

// applyTint sets the glass's tintColor from w.tint. It runs in a pool.
func (w *window) applyTint() {
	if w.effect == 0 || !w.glass {
		return
	}
	var color uintptr
	if w.tint != nil {
		color = uintptr(nsColor(*w.tint))
	}
	send(w.effect, "setTintColor:", color)
}

// removeEffect takes the material out of the window.
func (w *window) removeEffect() {
	if w.effect != 0 {
		send(w.effect, "removeFromSuperview")
		release(w.effect)
		w.effect, w.glass = 0, false
		w.restoreBacking()
	}
}

// clearForGlass makes a window that is not transparent clear while it
// shows the glass. NSGlassEffectView draws what is behind it in the
// window, which is the window's own color in an opaque one, where
// NSVisualEffectView blends with what is behind the window. The color of
// a rounded window is on its content view (roundContent). A popover's
// window is AppKit's.
func (w *window) clearForGlass() {
	if w.glassClear || w.opts.Transparent || w.popover != 0 {
		return
	}
	w.glassClear = true
	if w.radius > 0 {
		layer := send(w.view, "layer")
		if c := send(layer, "backgroundColor"); c != 0 && !w.systemBacking {
			w.backing = retain(send(class("NSColor"), "colorWithCGColor:", uintptr(c)))
		}
		send(layer, "setBackgroundColor:", 0)
	} else {
		w.backing = retain(send(w.win, "backgroundColor"))
		send(w.win, "setOpaque:", 0)
		send(w.win, "setBackgroundColor:", uintptr(send(class("NSColor"), "clearColor")))
	}
	send(w.win, "invalidateShadow")
}

// restoreBacking gives the window back the color clearForGlass took.
func (w *window) restoreBacking() {
	if !w.glassClear {
		return
	}
	w.glassClear = false
	switch {
	case w.radius > 0 && w.systemBacking:
		w.fillSystemBacking()
	case w.radius > 0:
		send(send(w.view, "layer"), "setBackgroundColor:", uintptr(send(w.backing, "CGColor")))
	default:
		send(w.win, "setBackgroundColor:", uintptr(w.backing))
		send(w.win, "setOpaque:", 1)
	}
	release(w.backing)
	w.backing = 0
	send(w.win, "invalidateShadow")
}

func (w *window) SetMenu(*platform.Menu) {}

// SetAutoHideMenu does nothing: the menu bar belongs to the application.
func (w *window) SetAutoHideMenu(bool) {}

// hiddenTitleBar reports a TitleBarStyle that hides the title bar but keeps
// the traffic lights.
func (w *window) hiddenTitleBar() bool {
	o := w.opts
	return !o.Frameless && (o.TitleBarStyle == "hidden" || o.TitleBarStyle == "hiddenInset")
}

// TitleBar returns the room the traffic lights take, from the left edge to
// the right edge of the zoom button and from the top edge to their bottom
// edge, in the title bar, which is as tall as the window's own or, with
// TrafficLightPosition, leaves as much room below them as above. They hide
// in full screen, and with HideWindowButtons.
func (w *window) TitleBar() platform.TitleBar {
	if !w.hiddenTitleBar() || w.closed || w.IsFullScreen() || w.opts.HideWindowButtons {
		return platform.TitleBar{}
	}
	closeBtn := send(w.win, "standardWindowButton:", 0)
	zoom := send(w.win, "standardWindowButton:", 2)
	if closeBtn == 0 || zoom == 0 {
		return platform.TitleBar{}
	}
	c, z := msgRect(closeBtn, sel("frame")), msgRect(zoom, sel("frame"))
	if p := w.trafficLights; p != nil {
		// Where layoutTrafficLights puts them, which may not have run yet.
		right := float64(p.X) + z.Origin.X - c.Origin.X + z.Size.Width
		return platform.TitleBar{
			Height: int(math.Round(c.Size.Height + 2*float64(p.Y))),
			Left:   int(math.Ceil(right)),
			Bottom: int(math.Ceil(float64(p.Y) + c.Size.Height)),
		}
	}
	frame := msgRect(w.win, sel("frame"))
	content := msgRect(w.win, sel("contentLayoutRect"))
	height := frame.Size.Height - content.Origin.Y - content.Size.Height
	// The close button in the window's coordinates, whose origin is the
	// bottom-left corner of its frame.
	lights := msgConvertRectView(closeBtn, sel("convertRect:toView:"), NSRect{Size: c.Size}, 0)
	return platform.TitleBar{
		Height: int(math.Round(height)),
		Left:   int(math.Ceil(z.Origin.X + z.Size.Width)),
		Bottom: int(math.Ceil(frame.Size.Height - lights.Origin.Y)),
	}
}

func (w *window) TitleBarDoubleClicked() {
	var action string
	withPool(func() {
		defaults := send(class("NSUserDefaults"), "standardUserDefaults")
		action = goString(send(defaults, "stringForKey:", uintptr(nsString("AppleActionOnDoubleClick"))))
	})
	switch action {
	case "None":
	case "Minimize":
		send(w.win, "performMiniaturize:", 0)
	default:
		send(w.win, "performZoom:", 0)
	}
}

func (w *window) Close() {
	if w.host != nil {
		w.closeEmbeddedView()
		return
	}
	if w.closed {
		return
	}
	if w.popover != 0 {
		w.closePopover()
		return
	}
	if w.sheet {
		w.sheet = false
		send(w.parent.win, "endSheet:", uintptr(w.win))
	}
	if w.opts.Flyout != nil {
		w.hideFlyout()
	}
	send(w.win, "close")
}

// layoutTrafficLights puts the top-left corner of the close button at the
// requested position, and the other buttons after it, in a title bar that
// leaves as much room below them as above. AppKit lays the title bar out
// again when the window resizes, its title changes or the appearance
// changes, so this runs after each of them.
func (w *window) layoutTrafficLights() {
	p := w.trafficLights
	if p == nil || w.closed || w.IsFullScreen() {
		return
	}
	closeBtn := send(w.win, "standardWindowButton:", 0)
	if closeBtn == 0 {
		return
	}
	container := send(send(closeBtn, "superview"), "superview")
	if container == 0 {
		return
	}
	btnFrame := msgRect(closeBtn, sel("frame"))
	height := btnFrame.Size.Height + 2*float64(p.Y)
	winFrame := msgRect(w.win, sel("frame"))
	cf := msgRect(container, sel("frame"))
	cf.Size.Height = height
	cf.Origin.Y = winFrame.Size.Height - height
	msgSetRect(container, sel("setFrame:"), cf)

	mini := send(w.win, "standardWindowButton:", 1)
	gap := msgRect(mini, sel("frame")).Origin.X - btnFrame.Origin.X
	for i := uintptr(0); i < 3; i++ {
		btn := send(w.win, "standardWindowButton:", i)
		msgSetPoint(btn, sel("setFrameOrigin:"), NSPoint{float64(p.X) + float64(i)*gap, float64(p.Y)})
	}
}

func primaryScreenHeight() float64 {
	screens := send(class("NSScreen"), "screens")
	if sendInt(screens, "count") == 0 {
		return 0
	}
	return msgRect(send(screens, "objectAtIndex:", 0), sel("frame")).Size.Height
}

func rectFromMac(r NSRect) platform.Rect {
	h := primaryScreenHeight()
	return platform.Rect{
		X:      int(math.Round(r.Origin.X)),
		Y:      int(math.Round(h - r.Origin.Y - r.Size.Height)),
		Width:  int(math.Round(r.Size.Width)),
		Height: int(math.Round(r.Size.Height)),
	}
}

func rectToMac(r platform.Rect) NSRect {
	h := primaryScreenHeight()
	return NSRect{
		Origin: NSPoint{float64(r.X), h - float64(r.Y) - float64(r.Height)},
		Size:   NSSize{float64(r.Width), float64(r.Height)},
	}
}

func (b *Backend) windowFor(delegate id) *window {
	w := b.byDelegate[delegate]
	if w == nil || w.closed {
		return nil
	}
	return w
}

func registerWindowClasses() {
	// Key view loop leaves an embedded view under native UI back to the surface.
	selectKeyView := func(class string, back bool) func(id, objc.SEL, id) {
		return func(self id, cmd objc.SEL, view id) {
			if w := embeddedViewOf(view); w != nil && w.host != nil {
				w.tabOut(back)
				return
			}
			sendSuper(self, class, cmd, uintptr(view))
		}
	}
	classDef("MyGoWindow", "NSWindow", nil, []objc.MethodDef{
		method("orderWindow:relativeTo:", orderWindowRelativeTo("MyGoWindow")),
		method("canBecomeKeyWindow", func(self id, _ objc.SEL) bool { return true }),
		method("canBecomeMainWindow", func(self id, _ objc.SEL) bool { return true }),
		method("selectKeyViewFollowingView:", selectKeyView("MyGoWindow", false)),
		method("selectKeyViewPrecedingView:", selectKeyView("MyGoWindow", true)),
	})
	// The theme frame of a titled window with corners of its own
	// (corners.go).
	frameClass := func(cls, base string) {
		class(cls).Class().AddMethod(sel("frameViewClassForStyleMask:"), objc.NewIMP(frameViewClassFor(base)), "#24@0:8Q16")
	}
	frameClass("MyGoWindow", "NSWindow")

	// A panel (WindowOptions.Panel) takes the keyboard, without
	// activating the app, and is never the main window.
	classDef("MyGoPanel", "NSPanel", nil, []objc.MethodDef{
		method("orderWindow:relativeTo:", orderWindowRelativeTo("MyGoPanel")),
		method("canBecomeKeyWindow", func(self id, _ objc.SEL) bool { return true }),
		method("canBecomeMainWindow", func(self id, _ objc.SEL) bool { return false }),
		// NSPanel closes on an Escape its content leaves (cancelOperation:),
		// past windowShouldClose: MyGo closes it then, as the user closing
		// it, which OnClose listeners may prevent.
		method("cancelOperation:", func(self id, _ objc.SEL, sender id) {}),
		method("selectKeyViewFollowingView:", selectKeyView("MyGoPanel", false)),
		method("selectKeyViewPrecedingView:", selectKeyView("MyGoPanel", true)),
	})
	frameClass("MyGoPanel", "NSPanel")

	// A flyout's panel becomes the key window only when it takes the
	// keyboard, and never the main window: its parent stays it.
	classDef("MyGoFlyout", "NSPanel", nil, []objc.MethodDef{
		method("canBecomeKeyWindow", func(self id, _ objc.SEL) bool {
			w := theBackend.byNSWindow[self]
			return w != nil && w.flyout.Focusable
		}),
		method("canBecomeMainWindow", func(self id, _ objc.SEL) bool { return false }),
		method("selectKeyViewFollowingView:", selectKeyView("MyGoFlyout", false)),
		method("selectKeyViewPrecedingView:", selectKeyView("MyGoFlyout", true)),
	})

	registerClipViewClass()

	b := func() *Backend { return theBackend }
	classDef("MyGoWindowDelegate", "NSObject",
		[]string{"NSWindowDelegate", "NSPopoverDelegate"},
		append(popoverDelegateMethods(), []objc.MethodDef{
			// NSWindowDelegate
			method("windowShouldClose:", func(self id, _ objc.SEL, sender id) bool {
				if w := b().windowFor(self); w != nil {
					return w.h.ShouldClose()
				}
				return true
			}),
			method("windowWillClose:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.cleanup()
					w.h.Closed()
				}
			}),
			method("windowDidBecomeKey:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.orderChildrenLater()
					w.h.Focused()
					w.surfaceKeyChanged(true)
				}
			}),
			method("windowDidResignKey:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.h.Blurred()
					w.surfaceKeyChanged(false)
					if w.opts.Flyout != nil && w.popover == 0 {
						// Once AppKit made another window key, or none,
						// unless the press on its tray icon toggles it.
						if t, ok := w.flyout.Tray.(*tray); !ok || !t.pressed() {
							b().post(w.dismissFlyout)
						}
					}
				}
			}),
			method("windowDidResize:", func(self id, _ objc.SEL, n id) {
				w := b().windowFor(self)
				if w == nil {
					return
				}
				w.layoutTrafficLights()
				if z := w.IsMaximized(); z != w.maximized {
					w.maximized = z
					if z {
						w.h.Maximized()
					} else {
						w.h.Unmaximized()
					}
				}
				if inputRegionMoved != nil {
					inputRegionMoved(w) // the region moved under the pointer
				}
				w.h.Resized()
			}),
			method("windowWillMove:", func(self id, _ objc.SEL, n id) {
				// The window server begins to move the window: a drag
				// region's press is its (window_drag.go).
				if w := b().windowFor(self); w != nil {
					theDrag.willMove(w.win)
				}
			}),
			method("windowDidMove:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					if inputRegionMoved != nil {
						inputRegionMoved(w)
					}
					w.h.Moved()
				}
			}),
			method("windowDidChangeOcclusionState:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil && w.surface != nil {
					if w.popover != 0 {
						// Its popover's window, from the notification
						// center (adoptPopoverWindow).
						w.tellPopoverOcclusion()
						return
					}
					kind := platform.SurfaceShown
					if !w.surface.visible() {
						kind = platform.SurfaceHidden
					}
					w.surface.send(platform.SurfaceEvent{Kind: kind})
				}
			}),
			method("windowDidMiniaturize:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.hideChildren()
					w.h.Minimized()
				}
			}),
			method("windowDidDeminiaturize:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.hidMiniaturized = false
					w.orderChildrenLater()
					w.h.Restored()
				}
			}),
			// The toolbar that insets the traffic lights holds no items: in
			// full screen it hides with the menu bar instead of covering the
			// top of the page.
			method("window:willUseFullScreenPresentationOptions:", func(self id, _ objc.SEL, win id, proposed uint) uint {
				const autoHide = presentationFullScreen | presentationAutoHideMenuBar
				if send(win, "toolbar") != 0 && proposed&autoHide == autoHide {
					return proposed | presentationAutoHideToolbar
				}
				return proposed
			}),
			// The child windows follow their parent into its full-screen
			// Space, and are left where they are as it comes out of it.
			method("windowWillEnterFullScreen:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.childrenAttached = true
					w.attachChildren()
				}
			}),
			method("windowDidFailToEnterFullScreen:", func(self id, _ objc.SEL, win id) {
				if w := b().windowFor(self); w != nil {
					w.leftFullScreen()
				}
			}),
			method("windowDidEnterFullScreen:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.h.EnteredFullScreen()
					if w.hiddenTitleBar() {
						w.h.TitleBarChanged() // the traffic lights hid
					}
				}
			}),
			method("windowDidExitFullScreen:", func(self id, _ objc.SEL, n id) {
				if w := b().windowFor(self); w != nil {
					w.leftFullScreen()
					w.layoutTrafficLights()
					w.h.LeftFullScreen()
					if w.hiddenTitleBar() {
						w.h.TitleBarChanged()
					}
				}
			}),

			method("observeValueForKeyPath:ofObject:change:context:", func(self id, _ objc.SEL, keyPath, object, change id, ctx uintptr) {
				w := b().windowFor(self)
				if w != nil && goString(keyPath) == "effectiveAppearance" {
					// AppKit lays the title bar out again after telling us.
					b().post(w.layoutTrafficLights)
				}
			}),
		}...))
}
