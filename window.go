package mygo

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// TitleBarStyle selects how the title bar of a framed window looks.
type TitleBarStyle string

// Title bar styles.
const (
	TitleBarDefault TitleBarStyle = "default"
	// TitleBarHidden gives the page the whole window while keeping the
	// window controls, which sit over it (see WindowOptions.TitleBarStyle).
	TitleBarHidden TitleBarStyle = "hidden"
	// TitleBarHiddenInset is TitleBarHidden with the window controls inset
	// further from the edges (macOS; TitleBarHidden elsewhere).
	TitleBarHiddenInset TitleBarStyle = "hiddenInset"
)

// DevTools selects whether the web inspector is available.
type DevTools int

// DevTools modes.
const (
	// DevToolsAuto enables the inspector unless the app was built for
	// production with `mygo build`.
	DevToolsAuto DevTools = iota
	DevToolsEnabled
	DevToolsDisabled
)

// Point is a position in screen coordinates.
type Point struct{ X, Y int }

// Size is a width and height in DIPs.
type Size struct{ Width, Height int }

// Rectangle is a position and size in screen coordinates (DIPs, origin at
// the top-left corner of the primary display).
type Rectangle struct{ X, Y, Width, Height int }

// WindowOptions configures NewWindow. The zero value is a visible,
// resizable 800x600 window centered on screen.
type WindowOptions struct {
	// OwnScene opens the window as a window of the system's own on iPad, beside
	// the app's others, which ios.multipleWindows in mygo.json allows. Elsewhere
	// it's a window as any: on a phone or Android, over the one before.
	OwnScene bool
	// Title of the window. Defaults to the application name.
	Title string

	// Width and Height of the window in DIPs (default 800x600).
	Width, Height int
	// UseContentSize makes Width and Height describe the content area instead
	// of the whole window.
	UseContentSize bool
	// X and Y of the top-left corner. The window is centered when both are
	// zero.
	X, Y                int
	MinWidth, MinHeight int
	MaxWidth, MaxHeight int

	// Hidden creates the window without showing it. Call Show, typically
	// from OnReadyToShow, to avoid a blank window.
	Hidden bool
	// ConsumeActivationClick consumes the click that makes a non-key
	// window key, without sending the press to its content (macOS,
	// Windows). It applies to the page, native UI and embedded WebViews;
	// system window controls keep their own behavior, while custom
	// title-bar drag regions (`--app-region: drag`, `ui.Element.DragWindow`)
	// require the window to be active first. On Windows, a regular
	// WebView2 page (without Content) disables its child control while
	// inactive so the activation click reaches the host window, which
	// also prevents wheel scrolling, hover and file drops on that page
	// until the window is active; native UI and embedded WebViews keep
	// all three while inactive. By default the activation click also
	// reaches the content. Other platforms ignore it.
	ConsumeActivationClick bool
	// Frameless removes the title bar and window chrome. Mark draggable
	// areas with the CSS `--app-region: drag`.
	Frameless bool
	// TitleBarStyle hides the title bar but keeps the window controls, so
	// the page fills the window and draws its own title bar under them:
	// the traffic lights on macOS, and minimize, maximize and close at the
	// top corner on Linux and Windows. On Linux they are GTK's own title
	// buttons, and the desktop's button layout decides which show and on
	// which side, possibly none. Pages keep clear of them with the
	// --mygo-titlebar-* CSS variables and drag the window by their title
	// bar with --app-region: drag, as in a frameless window; native UI
	// with ui.Context.TitleBar and ui.Element.DragWindow.
	TitleBarStyle TitleBarStyle
	// TrafficLightPosition moves the window controls of a window with a
	// hidden title bar (macOS): the top-left corner of the close button
	// goes this far from the top-left corner of the window.
	TrafficLightPosition *Point
	// TitleBarHeight is the height of the title bar that the page of a
	// window with a hidden title bar draws (Linux, Windows): the window
	// controls fill it on Windows and are centered in it on Linux. Zero is
	// 32 on Windows, as Windows 11's own title bars, and the height of the
	// desktop's header bars on Linux.
	TitleBarHeight int
	// HideWindowButtons hides the window controls of a window with a hidden
	// title bar (TitleBarStyle), which keeps the rest of it: the shadow,
	// the rounded corners and the resizing edges, the top one included,
	// which a Frameless window lacks on Windows. The page or the view draws
	// its own controls, anywhere, and calls Minimize, Maximize, Unmaximize
	// and Close; the --mygo-titlebar-* CSS variables and
	// ui.Context.TitleBar are then zero.
	HideWindowButtons bool

	DisableResize     bool
	DisableMove       bool
	DisableMinimize   bool
	DisableMaximize   bool
	DisableClose      bool
	DisableFullScreen bool
	DisableShadow     bool
	AlwaysOnTop       bool
	FullScreen        bool
	// Maximized creates the window maximized.
	Maximized bool
	// SkipTaskbar hides the window from the taskbar (Linux, Windows).
	SkipTaskbar bool
	// AutoHideMenuBar keeps the window's menu bar out of sight until the
	// user presses Alt or F10, and hides it again once they leave it
	// (Linux, Windows). Its keyboard shortcuts work all along.
	AutoHideMenuBar bool
	// Transparent makes the window background transparent, so a page with a
	// transparent background shows the desktop through.
	Transparent bool
	// BackgroundColor fills the window until the page paints, which avoids
	// a flash of another color while it loads. CSS syntax: "#1e1e1e",
	// "#rgba", "rgb(30 30 30)", or "light-dark(#f5f5f7, #1e1e1e)" for a
	// page that follows the light or dark appearance.
	BackgroundColor string
	// Vibrancy puts a translucent, blurred material behind a transparent
	// page, or behind native UI where it draws no background (macOS and
	// Windows 11 22H2), e.g. VibrancySidebar; ui.Context.Vibrancy tells
	// native UI whether it shows. On Windows, only a window created with a
	// material can show one, and it has no menu bar: Alt and F10 open its
	// menus in a popup.
	Vibrancy Vibrancy
	// VibrancyTint tints the material, in the syntax of BackgroundColor:
	// "#80ff0000", "rgba(255, 0, 0, 0.5)", or "light-dark(...)" to follow
	// the appearance. Only macOS 26's Liquid Glass takes a tint; other
	// materials and platforms ignore it.
	VibrancyTint string
	// CornerRadius rounds a frameless window's corners on macOS, in DIPs:
	// its content and material are cut to them, and its shadow follows
	// them, as a launcher's or a floating panel's. The window need not be
	// Transparent: its BackgroundColor fills its content, inside the
	// corners. On a titled window, macOS draws its corners at this radius
	// in place of the system's, and still widens them near the rounded
	// corners of a display, as it does its own. Windows 11 rounds a
	// frameless window given one with the corners of its own windows,
	// whatever the radius; Linux does not round it.
	CornerRadius int
	// Opacity of the window between 0 and 1 (default 1).
	Opacity float64
	// IgnoreMouseEvents creates the window letting the mouse through to
	// what is behind it, as SetIgnoreMouseEvents(true) does: an overlay
	// that marks a part of the screen never takes a click, not even
	// between its creation and that call.
	IgnoreMouseEvents bool
	// Parent makes this window a child window that stays on top of it
	// and closes with it. It can go to any display, and stays where it is
	// as its parent moves. On macOS it is ordered above its parent
	// whenever the parent comes in front, stays above a parent that is
	// AlwaysOnTop, goes with it into full screen, and hides while the
	// parent is minimized or hidden. The Parent may be a flyout, a tray's
	// included: without X and Y, the window is centered over it, in the
	// work area of its display, and stays above it.
	Parent *Window
	// Modal makes a child window modal to its Parent, which takes no
	// input until it closes: a sheet of the parent on macOS, unless the
	// parent is a flyout. A flyout stays open while a window is modal to
	// it, wherever the keyboard goes, and closing the flyout closes the
	// window: a confirmation a tray's flyout asks for, wider than it.
	Modal bool

	// Panel makes the window a panel, as the windows of launchers and
	// quick entry (Spotlight, Raycast) are: it floats above the windows of
	// all apps, on whichever Space or virtual desktop the user is on, over
	// full-screen apps too, and stays out of the taskbar, the Dock and the
	// app switcher. It cannot be minimized, maximized or go full screen. On
	// macOS, showing or focusing it gives it the keyboard without
	// activating the app, so the app the user was in stays active and gets
	// the keyboard back when the panel hides.
	Panel bool

	// DevTools selects whether the native UI inspector is available.
	DevTools DevTools

	// StateKey remembers the window's position, size, and maximized and
	// full screen state under this key, in window-state.json in
	// PathUserData or the store of App.SetWindowStateStore, saved when
	// the window closes and when the app quits. A window created with the
	// same key, for example at the next launch, gets them back as long as
	// it would show on a connected display: they override X, Y, Width,
	// Height, UseContentSize, Maximized and FullScreen.
	// App.ClearWindowStates forgets them.
	StateKey string

	// Content makes the window show native UI that MyGo draws itself, on
	// the GPU where it can, built with package ui.
	Content Content
}

// WindowState is what WindowOptions.StateKey remembers of a window: its
// bounds in the normal state, which it gets back when it leaves the
// maximized or full screen state, and that state.
type WindowState struct {
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Maximized  bool `json:"maximized,omitempty"`
	FullScreen bool `json:"fullScreen,omitempty"`
	// PixelX, PixelY, PixelWidth and PixelHeight are the normal bounds in
	// the screen's pixels, which Windows keeps: where displays have
	// different scales, X and Y change with the scales, and earlier
	// versions' cannot tell the displays apart. Without them, a window is
	// centered there when the displays have different scales.
	PixelX      int `json:"pixelX,omitzero"`
	PixelY      int `json:"pixelY,omitzero"`
	PixelWidth  int `json:"pixelWidth,omitzero"`
	PixelHeight int `json:"pixelHeight,omitzero"`
}

// WindowStateStore keeps the states of the windows with a
// WindowOptions.StateKey, by key, for App.SetWindowStateStore. MyGo calls
// it on the main thread, so it should return quickly. It keeps every
// field of a state, as JSON does: on Windows, a window whose state lost
// its pixels (WindowState.PixelX) is centered when the displays have
// different scales.
//
// In a quit, the states are saved after the OnBeforeQuit listeners have
// run, as each window closes, and once more after the event loop has
// stopped, right before the OnQuit listeners run: states that the app
// removes from the store itself in OnBeforeQuit are written again.
// App.ClearWindowStates drops them for good instead, from any of those
// listeners or at any other time, through SaveWindowStates with no
// states.
type WindowStateStore interface {
	// LoadWindowStates returns the saved states. It is called once, when
	// the first window with a StateKey is created. Without states (nil, or
	// an error) windows get their options.
	LoadWindowStates() (map[string]WindowState, error)
	// SaveWindowStates replaces the saved states with states, which the
	// store may keep. It is called when a window with a StateKey closes
	// and when the app quits, if a state changed, and with an empty map
	// by App.ClearWindowStates. After an error the next call saves the
	// states again.
	SaveWindowStates(states map[string]WindowState) error
}

// SetWindowStateStore keeps the states of windows (WindowOptions.StateKey)
// in s, such as the app's own database, instead of window-state.json in
// PathUserData. nil restores the file. Call it before the first window
// with a StateKey is created, usually before Run. Browsers keep no window
// state, and never call s.
func (a *Application) SetWindowStateStore(s WindowStateStore) {
	a.mu.Lock()
	a.stateStore = s
	a.mu.Unlock()
}

// ClearWindowStates forgets the states of the windows
// (WindowOptions.StateKey), as an app's "Reset settings" does: it saves
// no states to the store (window-state.json, or the store of
// SetWindowStateStore, with an empty map), and the windows save theirs no
// more for the rest of the run. The windows open keep their bounds; those
// created afterwards, as those of the next launch, get their options.
// It may be called at any time, from any goroutine, from the OnBeforeQuit,
// OnWillQuit and OnQuit listeners too, which run before the quit's last
// save, and before Run. It returns the store's error; the next save, when
// a window closes or the app quits, tries again.
func (a *Application) ClearWindowStates() error { return onMainValue(clearWindowStates) }

// Window is a native window hosting a web page, or the Content of package
// ui. Create windows with NewWindow once the application is ready.
type Window struct {
	id     int
	parent *Window
	// flyout places a window NewFlyout created, at flyoutSize, the size
	// it asked for. Main thread only.
	flyout     *platform.Flyout
	flyoutSize platform.Size
	// panel is WindowOptions.Panel.
	panel bool
	// cornerRadius is WindowOptions.CornerRadius: the content rounds its
	// own corners to it where the system does not (Windows 10).
	cornerRadius int
	// transparent is WindowOptions.Transparent: the window shows what is
	// behind it, so that the arc of its corners has something to show.
	// frameless is WindowOptions.Frameless.
	transparent, frameless bool
	// active tells that the window is drawn as active (active.go);
	// focusKnown, that it had the keyboard once or showed without it;
	// keepActive, that SetActiveAppearance keeps it active. Main thread
	// only.
	active, focusKnown, keepActive bool
	// focusedAt orders the windows by when they last had the keyboard,
	// from focusCount, 0 for never: raiseApp shows the latest. Main
	// thread only.
	focusedAt uint64
	// fit sizes a flyout that takes its content's size (FitContent), nil
	// for others. Main thread only.
	fit *fitContent
	// content tells that the window shows WindowOptions.Content, which
	// the window hands to its surface as it attaches it and keeps no
	// reference to: the Content's view, and all it reaches, stays only as
	// long as the window is open (#1281). conn connects it to the native
	// surface (main thread only). devTools tells that DevTools turned the
	// native inspector on.
	content      bool
	conn         *surface.Conn
	devTools     bool
	invalidating atomic.Bool

	// native is only accessed on the main thread; nil once destroyed.
	native    platform.Window
	destroyed atomic.Bool
	shown     bool
	readyShow bool
	// showLater tells that NewWindow left showing the window of native UI
	// to the main loop's next turn (settleShow), and framing that its
	// content draws a frame. Main thread only.
	showLater, framing bool
	// background is the BackgroundColor, if any. Main thread only.
	background *background
	// vibrancyTint is VibrancyTint, if any. Main thread only.
	vibrancyTint *background
	// hiddenTitleBar is a TitleBarStyle that hides the title bar. Main thread only.
	hiddenTitleBar bool
	menu           *Menu
	// thumbBar is what SetThumbBar set last; a button's Click is looked up
	// here by the position the shell reports. Main thread only.
	thumbBar []ThumbButton
	// title is the title the app set; representedFile and documentEdited are
	// the document the window shows. Main thread only.
	title           string
	representedFile string
	documentEdited  bool
	// stateKey is WindowOptions.StateKey; stateTimer captures the state
	// once it settled. Main thread only.
	stateKey   string
	stateTimer *time.Timer
	// devSlot is the place, from 1, of a window without a stateKey in
	// what mygo dev keeps of the windows (restoreDevWindow), 0 for none.
	// Main thread only.
	devSlot int

	onClose           listeners[func(*CloseEvent)]
	onClosed          listeners[func()]
	onFocus           listeners[func()]
	onBlur            listeners[func()]
	onShow            listeners[func()]
	onHide            listeners[func()]
	onReadyToShow     listeners[func()]
	onResize          listeners[func()]
	onFileDrop        listeners[func(*FileDropEvent)]
	onMove            listeners[func()]
	onMaximize        listeners[func()]
	onUnmaximize      listeners[func()]
	onMinimize        listeners[func()]
	onRestore         listeners[func()]
	onEnterFullScreen listeners[func()]
	onLeaveFullScreen listeners[func()]
}

var windows struct {
	sync.Mutex
	nextID  int
	list    []*Window
	focused *Window
}

// focusCount counts the times a window took the keyboard
// (Window.focusedAt). Main thread only.
var focusCount uint64

// NewWindow creates a window. It must be called after the application is
// ready (see Application.WhenReady); when called from another goroutine it
// waits for that.
func NewWindow(opts WindowOptions) *Window {
	// Validate in the caller's goroutine so mistakes point at the call.
	bg, err := backgroundOption(opts.BackgroundColor)
	if err != nil {
		panic(err)
	}
	tint, err := backgroundOption(opts.VibrancyTint)
	if err != nil {
		panic(err)
	}
	return createWindow("NewWindow", opts, bg, tint, nil)
}

// createWindow creates a window, or a flyout, once the application is
// ready.
func createWindow(caller string, opts WindowOptions, bg, tint *background, flyout *flyoutOptions) *Window {
	main := isMainThread()
	if !main {
		App.waitReady()
	} else if !App.IsReady() {
		panic("mygo: " + caller + " called before the application is ready; create windows in App.WhenReady")
	}
	var w *Window
	onMain(func() {
		w = newWindow(opts, bg, tint, flyout)
		if !main {
			// Called from another goroutine, NewWindow returns while the
			// main thread goes on drawing: the window of native UI shows
			// before it does.
			w.settleShow()
		}
	})
	if w == nil {
		// The event loop has already stopped.
		w = &Window{}
		w.destroyed.Store(true)
	}
	return w
}

func backgroundOption(s string) (*background, error) {
	if s == "" {
		return nil, nil
	}
	bg, err := parseBackground(s)
	if err != nil {
		return nil, err
	}
	return &bg, nil
}

// backgroundColor returns the window's background for the current
// appearance, or nil. Main thread only.
func (w *Window) backgroundColor() *platform.Color {
	if w.background == nil {
		return nil
	}
	c := w.background.light
	if w.background.dark != c && backend().Theme().IsDark() {
		c = w.background.dark
	}
	return &c
}

// vibrancyTintColor returns the window's material tint for the current
// appearance, or nil. Main thread only.
func (w *Window) vibrancyTintColor() *platform.Color {
	if w.vibrancyTint == nil {
		return nil
	}
	c := w.vibrancyTint.light
	if w.vibrancyTint.dark != c && backend().Theme().IsDark() {
		c = w.vibrancyTint.dark
	}
	return &c
}

// updateBackgrounds gives windows with a light and a dark background the
// one of the current appearance. Main thread only.
func updateBackgrounds() {
	for _, w := range Windows() {
		if bg := w.background; w.native != nil && bg != nil && bg.light != bg.dark {
			w.native.SetBackgroundColor(*w.backgroundColor())
		}
		if tint := w.vibrancyTint; w.native != nil && tint != nil && tint.light != tint.dark {
			w.native.SetVibrancyTint(w.vibrancyTintColor())
		}
	}
}

func newWindow(opts WindowOptions, bg, tint *background, flyout *flyoutOptions) *Window {
	windows.Lock()
	windows.nextID++
	id := windows.nextID
	windows.Unlock()

	w := &Window{id: id, parent: opts.Parent, stateKey: opts.StateKey, background: bg, vibrancyTint: tint, content: opts.Content != nil, panel: opts.Panel && flyout == nil, active: true, cornerRadius: opts.CornerRadius, transparent: opts.Transparent, frameless: opts.Frameless}
	if flyout != nil {
		w.fit = flyout.fit
	}
	w.hiddenTitleBar = !opts.Frameless && (opts.TitleBarStyle == TitleBarHidden || opts.TitleBarStyle == TitleBarHiddenInset)
	popts := w.platformOptions(&opts)
	popts.BackgroundColor = w.backgroundColor()
	popts.Surface = opts.Content != nil
	popts.OwnScene = opts.OwnScene
	if opts.Transparent {
		// Backends keep transparent windows clear; so do theme changes.
		w.background = nil
	}
	if flyout != nil {
		w.flyout = flyout.flyout
		if t := flyout.tray; t != nil {
			if t.native == nil {
				panic("mygo: NewFlyout: the tray was destroyed")
			}
			w.flyout.Tray = t.native
		}
		w.flyoutSize = platform.Size{Width: popts.Width, Height: popts.Height}
		popts.Flyout = w.flyout
		popts.Focusable = w.flyout.Focusable
		popts.Center = false
	}
	w.devTools = popts.DevTools
	if w.stateKey != "" {
		restoreWindowState(w.stateKey, popts)
	} else {
		w.restoreDevWindow(popts)
	}
	w.title = popts.Title
	nw, err := backend().NewWindow(popts, &windowHandler{w})
	if err != nil {
		panic(fmt.Sprintf("mygo: cannot create window: %v", err))
	}
	w.native = nw
	if opts.IgnoreMouseEvents {
		// Before the window shows, and on the turn that made it: no click
		// reaches it in between.
		nw.SetIgnoreMouseEvents(true)
	}
	if w.stateKey != "" || w.devSlot > 0 {
		w.initState(popts)
	}
	if w.content {
		w.attachContent(opts.Content)
	}

	windows.Lock()
	windows.list = append(windows.list, w)
	windows.Unlock()

	fire1(&App.onWindowCreated, w)
	switch {
	case opts.Hidden:
	case w.content:
		// The window of native UI shows, and its view runs for the first
		// frame, once NewWindow returned on the main thread, so that the
		// view can use what NewWindow returns (#1053): shown now, the
		// window would draw its first frame at once on some systems, as
		// AppKit may display a window it orders in and Windows a window
		// maximized as it shows. The frame the content asked for draws on
		// that turn as well, while the window is still hidden, as a
		// window made Hidden draws its first, or as it shows.
		w.showLater = true
		postMain(w.settleShow)
	default:
		w.Show()
	}
	return w
}

func (w *Window) platformOptions(o *WindowOptions) *platform.WindowOptions {
	p := &platform.WindowOptions{
		Title:                  o.Title,
		X:                      o.X,
		Y:                      o.Y,
		Width:                  or(o.Width, 800),
		Height:                 or(o.Height, 600),
		Center:                 o.X == 0 && o.Y == 0,
		UseContentSize:         o.UseContentSize,
		MinSize:                platform.Size{Width: o.MinWidth, Height: o.MinHeight},
		MaxSize:                platform.Size{Width: o.MaxWidth, Height: o.MaxHeight},
		Resizable:              !o.DisableResize,
		Movable:                !o.DisableMove,
		Minimizable:            !o.DisableMinimize,
		Maximizable:            !o.DisableMaximize,
		Closable:               !o.DisableClose,
		Focusable:              true,
		Fullscreenable:         !o.DisableFullScreen,
		AlwaysOnTop:            o.AlwaysOnTop,
		FullScreen:             o.FullScreen,
		Maximized:              o.Maximized,
		SkipTaskbar:            o.SkipTaskbar,
		AutoHideMenu:           o.AutoHideMenuBar,
		HasShadow:              !o.DisableShadow,
		Frameless:              o.Frameless,
		Transparent:            o.Transparent,
		TitleBarStyle:          string(o.TitleBarStyle),
		TitleBarHeight:         max(o.TitleBarHeight, 0),
		HideWindowButtons:      o.HideWindowButtons,
		Vibrancy:               string(o.Vibrancy),
		VibrancyTint:           w.vibrancyTintColor(),
		CornerRadius:           max(o.CornerRadius, 0),
		Opacity:                o.Opacity,
		Modal:                  o.Modal,
		Panel:                  o.Panel,
		ConsumeActivationClick: o.ConsumeActivationClick,
	}
	if o.Panel {
		p.Minimizable, p.Maximizable, p.Fullscreenable = false, false, false
		p.AlwaysOnTop, p.SkipTaskbar = true, true
	}
	if p.Title == "" {
		p.Title = App.Name()
	}
	if p.Opacity <= 0 || p.Opacity > 1 {
		p.Opacity = 1
	}
	switch o.DevTools {
	case DevToolsEnabled:
		p.DevTools = true
	case DevToolsAuto:
		p.DevTools = IsDev()
	}
	if o.TrafficLightPosition != nil {
		p.TrafficLightPosition = &platform.Point{X: o.TrafficLightPosition.X, Y: o.TrafficLightPosition.Y}
	}
	if o.Parent != nil && o.Parent.native != nil {
		p.Parent = o.Parent.native
		if o.Parent.flyout != nil && p.Center {
			p.Center = false
			p.X, p.Y = overFlyout(o.Parent.native.Bounds(), platform.Size{Width: p.Width, Height: p.Height})
		}
	}
	return p
}

// overFlyout returns where a window of size goes over a flyout at
// bounds: centered on it, and moved into the work area of its display,
// which a dialog wider than a flyout at the edge of the screen, as a
// tray's, would cross.
func overFlyout(bounds platform.Rect, size platform.Size) (x, y int) {
	r := platform.Rect{X: bounds.X + (bounds.Width-size.Width)/2, Y: bounds.Y + (bounds.Height-size.Height)/2, Width: size.Width, Height: size.Height}
	if wa := platform.WorkAreaFor(bounds, backend().Screen().Displays()); wa.Width > 0 && wa.Height > 0 {
		r.X = max(min(r.X, wa.X+wa.Width-r.Width), wa.X)
		r.Y = max(min(r.Y, wa.Y+wa.Height-r.Height), wa.Y)
	}
	return r.X, r.Y
}

func or[T comparable](v, fallback T) T {
	var zero T
	if v == zero {
		return fallback
	}
	return v
}

// Windows returns all open windows in creation order.
func Windows() []*Window {
	windows.Lock()
	defer windows.Unlock()
	return append([]*Window(nil), windows.list...)
}

// FocusedWindow returns the focused window of this application, or nil.
func FocusedWindow() *Window {
	windows.Lock()
	defer windows.Unlock()
	return windows.focused
}

// WindowByID returns the window with the given id, or nil.
func WindowByID(id int) *Window {
	windows.Lock()
	defer windows.Unlock()
	for _, w := range windows.list {
		if w.id == id {
			return w
		}
	}
	return nil
}

// ID returns the unique id of the window. The page sees it as
// window.mygo.windowId.
func (w *Window) ID() int { return w.id }

// IsDestroyed reports whether the window has been closed.
func (w *Window) IsDestroyed() bool { return w.destroyed.Load() }

// Parent returns the parent window, or nil.
func (w *Window) Parent() *Window { return w.parent }

// do runs fn with the native window on the main thread, unless the window
// has been destroyed.
func (w *Window) do(fn func(n platform.Window)) {
	onMain(func() {
		w.settleShow()
		if w.native != nil {
			fn(w.native)
		}
	})
}

// get is do for functions returning a value.
func get[T any](w *Window, fn func(n platform.Window) T) T {
	return onMainValue(func() T {
		w.settleShow()
		if w.native == nil {
			var zero T
			return zero
		}
		return fn(w.native)
	})
}

// settleShow shows the window that NewWindow left to show on the main
// loop's next turn, unless Show, ShowInactive or Hide came first: the
// methods acting on the native window show it first, so that they find it
// shown, as NewWindow returned it. Not from the first frame, drawn while
// the window is hidden, whose view may call them (Title): the system may
// draw a frame as the window shows, which would run within that one. Main
// thread only.
func (w *Window) settleShow() {
	if w.showLater && !w.framing {
		w.show()
	}
}

// Close closes the window as if the user clicked its close button, so
// OnClose listeners can cancel it.
func (w *Window) Close() { onMain(func() { w.close() }) }

// close runs on the main thread and reports whether the window was closed.
func (w *Window) close() bool {
	if w.native == nil {
		return true
	}
	e := &CloseEvent{Window: w}
	fire1(&w.onClose, e)
	if e.prevented {
		return false
	}
	w.destroy()
	return true
}

// Destroy closes the window without emitting OnClose.
func (w *Window) Destroy() { onMain(w.destroy) }

func (w *Window) destroy() {
	if n := w.native; n != nil {
		w.closeState()
		n.Close()
		// Backends report Closed synchronously, but be defensive.
		(&windowHandler{w}).Closed()
	}
}

// Show shows and focuses the window.
func (w *Window) Show() { onMain(w.show) }

func (w *Window) show() {
	if backgroundShow {
		w.showInactive()
		return
	}
	w.showLater = false
	if w.native == nil {
		return
	}
	visible := w.native.IsVisible()
	if w.fitShow(visible); w.native == nil {
		return // the frame it laid out closed it
	}
	w.native.Show()
	w.shown = true
	if !visible {
		fire(&w.onShow)
	}
}

// ShowInactive shows the window without focusing it.
func (w *Window) ShowInactive() { onMain(w.showInactive) }

func (w *Window) showInactive() {
	w.showLater = false
	if w.native == nil {
		return
	}
	visible := w.native.IsVisible()
	if w.fitShow(visible); w.native == nil {
		return // the frame it laid out closed it
	}
	w.native.ShowInactive()
	w.shown = true
	if !w.focusKnown {
		w.focusKnown = true
		scheduleActive()
	}
	if !visible {
		fire(&w.onShow)
	}
}

// backgroundShow tells that an app `mygo dev -background` launched
// (devBackground) has not come to the front yet: its windows show without
// the focus, those NewWindow shows and those the app shows itself, as one
// made Hidden until OnReadyToShow, since Show would bring the app there,
// until one of its windows takes the focus, as the user clicks it, or the
// app becomes the active one. Main thread only.
var backgroundShow = devBackground

// fitShow sizes a hidden flyout of native UI that takes its content's
// size to that content before it shows (fitContent.show). Main thread
// only.
func (w *Window) fitShow(visible bool) {
	if f := w.fit; f != nil && f.show != nil && !visible {
		f.show(w)
	}
}

// Hide hides the window. Called right after NewWindow, it keeps a window
// of native UI from showing at all.
func (w *Window) Hide() {
	onMain(func() {
		w.showLater = false
		if w.native == nil || !w.native.IsVisible() {
			return
		}
		w.native.Hide()
		fire(&w.onHide)
	})
}

// IsPanel reports whether the window was created with
// WindowOptions.Panel.
func (w *Window) IsPanel() bool { return w.panel }

// IsVisible reports whether the window is shown: Show or ShowInactive
// (or WindowOptions) showed it and Hide did not hide it since. A
// minimized window is still shown, on every platform, as is one on
// another desktop or covered by other windows. To ask whether it is
// minimized, use IsMinimized; whether any of a window of native UI shows
// on screen, Context.WindowHidden in its view.
func (w *Window) IsVisible() bool { return get(w, platform.Window.IsVisible) }

// Focus focuses the window.
func (w *Window) Focus() { w.do(platform.Window.Focus) }

// Blur removes focus from the window.
func (w *Window) Blur() { w.do(platform.Window.Blur) }

// IsFocused reports whether the window has keyboard focus.
func (w *Window) IsFocused() bool { return get(w, platform.Window.IsFocused) }

// CornerRadius returns the radius, in DIPs, that the window's corners are
// rounded to on screen, for content inset from its edges to round its own
// corners concentric with them: a card inset by d takes CornerRadius less
// d. On macOS it is the window's WindowOptions.CornerRadius, or the one
// the system draws the window with, which depends on the system
// and the title bar (on macOS 26, 26 with TitleBarHiddenInset's toolbar
// and 16 without one); on Windows 11, the 8 of its rounded windows. It is 0 where the corners are square (full screen, a maximized
// window on Windows, Linux, browsers) or the system does not tell it: a
// transparent frameless window on Windows 10 rounds its content's corners
// itself, at its WindowOptions.CornerRadius, as the system keeps them square.
func (w *Window) CornerRadius() float64 { return get(w, platform.Window.CornerRadius) }

// Minimize minimizes the window.
func (w *Window) Minimize() { w.do(platform.Window.Minimize) }

// IsMinimized reports whether the window is minimized.
func (w *Window) IsMinimized() bool { return get(w, platform.Window.IsMinimized) }

// Maximize maximizes the window.
func (w *Window) Maximize() { w.do(platform.Window.Maximize) }

// Unmaximize restores a maximized window.
func (w *Window) Unmaximize() { w.do(platform.Window.Unmaximize) }

// ToggleMaximize maximizes the window or restores it if it is maximized.
func (w *Window) ToggleMaximize() {
	w.do(func(n platform.Window) {
		if n.IsMaximized() {
			n.Unmaximize()
		} else {
			n.Maximize()
		}
	})
}

// IsMaximized reports whether the window is maximized.
func (w *Window) IsMaximized() bool { return get(w, platform.Window.IsMaximized) }

// Restore restores a minimized window.
func (w *Window) Restore() { w.do(platform.Window.Restore) }

// SetFullScreen enters or leaves full screen.
func (w *Window) SetFullScreen(v bool) {
	w.do(func(n platform.Window) { n.SetFullScreen(v) })
}

// ToggleFullScreen enters full screen or leaves it.
func (w *Window) ToggleFullScreen() {
	w.do(func(n platform.Window) { n.SetFullScreen(!n.IsFullScreen()) })
}

// IsFullScreen reports whether the window is in full screen.
func (w *Window) IsFullScreen() bool { return get(w, platform.Window.IsFullScreen) }

// Center moves the window to the center of its screen.
func (w *Window) Center() { w.do(platform.Window.Center) }

// SetTitle sets the native window title.
func (w *Window) SetTitle(title string) {
	w.do(func(n platform.Window) {
		w.title = title
		w.applyTitle(n)
	})
}

// Title returns the window title, without the mark of an edited document
// (see SetDocumentEdited).
func (w *Window) Title() string {
	return get(w, func(platform.Window) string { return w.title })
}

// SetRepresentedFile tells which file the window shows, "" for none. On
// macOS the title bar shows its icon, which the user can drag, and
// Command-clicking the title lists the folders it is in. Other platforms
// show nothing. Documents (NewDocuments) set it for their windows.
func (w *Window) SetRepresentedFile(path string) {
	if path != "" {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	w.do(func(n platform.Window) {
		w.representedFile = path
		n.SetRepresentedFile(path)
	})
}

// RepresentedFile returns the path SetRepresentedFile set.
func (w *Window) RepresentedFile() string {
	return onMainValue(func() string { return w.representedFile })
}

// SetDocumentEdited tells whether the window's document has unsaved
// changes: a dot in the close button on macOS, and a "*" before the title
// on Linux and Windows, as Notepad and gedit show it. Documents
// (NewDocuments) set it for their windows.
func (w *Window) SetDocumentEdited(edited bool) {
	w.do(func(n platform.Window) {
		if w.documentEdited == edited {
			return
		}
		w.documentEdited = edited
		n.SetDocumentEdited(edited)
		w.applyTitle(n)
	})
}

// IsDocumentEdited reports what SetDocumentEdited set.
func (w *Window) IsDocumentEdited() bool {
	return onMainValue(func() bool { return w.documentEdited })
}

// applyTitle gives the native window its title, marked when the document
// is edited where the title bar has no other mark. Main thread only.
func (w *Window) applyTitle(n platform.Window) {
	title := w.title
	if w.documentEdited && runtime.GOOS != "darwin" {
		title = "*" + title
	}
	n.SetTitle(title)
}

// SetBounds moves and resizes the window.
func (w *Window) SetBounds(r Rectangle) {
	w.do(func(n platform.Window) { n.SetBounds(platform.Rect(r)) })
}

// Bounds returns the position and size of the window.
func (w *Window) Bounds() Rectangle {
	return get(w, func(n platform.Window) Rectangle { return Rectangle(n.Bounds()) })
}

// SetContentBounds moves and resizes the window so the page area has the
// given bounds.
func (w *Window) SetContentBounds(r Rectangle) {
	w.do(func(n platform.Window) { n.SetContentBounds(platform.Rect(r)) })
}

// ContentBounds returns the bounds of the page area.
func (w *Window) ContentBounds() Rectangle {
	return get(w, func(n platform.Window) Rectangle { return Rectangle(n.ContentBounds()) })
}

// SetSize resizes the window.
func (w *Window) SetSize(width, height int) {
	w.do(func(n platform.Window) {
		if w.setFlyoutSize(width, height) {
			return
		}
		b := n.Bounds()
		b.Width, b.Height = width, height
		n.SetBounds(b)
	})
}

// Size returns the size of the window.
func (w *Window) Size() (width, height int) {
	b := w.Bounds()
	return b.Width, b.Height
}

// SetContentSize resizes the window so the page area has the given size,
// in the window's DIPs. A zoom does not change them: a page zoomed by z
// (Page.SetZoomFactor), or native UI (ui's Context.SetZoom), lays out in
// width/z × height/z of its own. Multiply a size the view needs by the
// zoom:
//
//	z := c.Zoom() // in native UI; Page.ZoomFactor for a page
//	win.SetContentSize(int(math.Ceil(float64(320*z))), int(math.Ceil(float64(400*z))))
func (w *Window) SetContentSize(width, height int) {
	w.do(func(n platform.Window) {
		if w.setFlyoutSize(width, height) {
			return
		}
		b := n.ContentBounds()
		b.Width, b.Height = width, height
		n.SetContentBounds(b)
	})
}

// ContentSize returns the size of the page area, in the window's DIPs:
// a view zoomed by z sees it divided by z, as ui's Context.Size returns it.
func (w *Window) ContentSize() (width, height int) {
	b := w.ContentBounds()
	return b.Width, b.Height
}

// SetPosition moves the window's top-left corner.
func (w *Window) SetPosition(x, y int) {
	w.do(func(n platform.Window) {
		b := n.Bounds()
		b.X, b.Y = x, y
		n.SetBounds(b)
	})
}

// Position returns the window's top-left corner.
func (w *Window) Position() (x, y int) {
	b := w.Bounds()
	return b.X, b.Y
}

// SetMinimumSize limits how small the window can be resized; 0 means no
// limit.
func (w *Window) SetMinimumSize(width, height int) {
	w.do(func(n platform.Window) { n.SetMinimumSize(platform.Size{Width: width, Height: height}) })
}

// SetMaximumSize limits how large the window can be resized; 0 means no
// limit.
func (w *Window) SetMaximumSize(width, height int) {
	w.do(func(n platform.Window) { n.SetMaximumSize(platform.Size{Width: width, Height: height}) })
}

// SetResizable sets whether the user can resize the window.
func (w *Window) SetResizable(v bool) { w.do(func(n platform.Window) { n.SetResizable(v) }) }

// IsResizable reports whether the user can resize the window.
func (w *Window) IsResizable() bool { return get(w, platform.Window.IsResizable) }

// SetMovable sets whether the user can move the window.
func (w *Window) SetMovable(v bool) { w.do(func(n platform.Window) { n.SetMovable(v) }) }

// IsMovable reports whether the user can move the window.
func (w *Window) IsMovable() bool { return get(w, platform.Window.IsMovable) }

// SetMinimizable sets whether the window can be minimized.
func (w *Window) SetMinimizable(v bool) { w.do(func(n platform.Window) { n.SetMinimizable(v) }) }

// IsMinimizable reports whether the window can be minimized.
func (w *Window) IsMinimizable() bool { return get(w, platform.Window.IsMinimizable) }

// SetMaximizable sets whether the window can be maximized.
func (w *Window) SetMaximizable(v bool) { w.do(func(n platform.Window) { n.SetMaximizable(v) }) }

// IsMaximizable reports whether the window can be maximized.
func (w *Window) IsMaximizable() bool { return get(w, platform.Window.IsMaximizable) }

// SetClosable sets whether the user can close the window.
func (w *Window) SetClosable(v bool) { w.do(func(n platform.Window) { n.SetClosable(v) }) }

// IsClosable reports whether the user can close the window.
func (w *Window) IsClosable() bool { return get(w, platform.Window.IsClosable) }

// SetAlwaysOnTop keeps the window above other windows.
func (w *Window) SetAlwaysOnTop(v bool) { w.do(func(n platform.Window) { n.SetAlwaysOnTop(v) }) }

// IsAlwaysOnTop reports whether the window stays above other windows.
func (w *Window) IsAlwaysOnTop() bool { return get(w, platform.Window.IsAlwaysOnTop) }

// SetBackgroundColor sets the color shown behind the page, in the syntax of
// WindowOptions.BackgroundColor.
func (w *Window) SetBackgroundColor(color string) error {
	bg, err := parseBackground(color)
	if err != nil {
		return err
	}
	w.do(func(n platform.Window) {
		w.background = &bg
		n.SetBackgroundColor(*w.backgroundColor())
	})
	return nil
}

// SetOpacity sets the window opacity between 0 and 1.
func (w *Window) SetOpacity(v float64) {
	w.do(func(n platform.Window) { n.SetOpacity(min(max(v, 0), 1)) })
}

// Opacity returns the window opacity.
func (w *Window) Opacity() float64 { return get(w, platform.Window.Opacity) }

// SetHasShadow sets whether the window has a shadow. On Linux a window
// keeps the shadow it was created with (DisableShadow).
func (w *Window) SetHasShadow(v bool) { w.do(func(n platform.Window) { n.SetHasShadow(v) }) }

// HasShadow reports whether the window has a shadow.
func (w *Window) HasShadow() bool { return get(w, platform.Window.HasShadow) }

// SetIgnoreMouseEvents makes the window transparent to mouse events. It
// wins over SetInputRegion.
func (w *Window) SetIgnoreMouseEvents(v bool) {
	w.do(func(n platform.Window) { n.SetIgnoreMouseEvents(v) })
}

// IgnoresMouseEvents reports whether the window lets the mouse through, as
// SetIgnoreMouseEvents or WindowOptions.IgnoreMouseEvents made it. A
// window with an input region (SetInputRegion) takes the mouse on a part
// of it, and does not ignore it.
func (w *Window) IgnoresMouseEvents() bool { return get(w, platform.Window.IgnoresMouseEvents) }

// SetInputRegion limits the part of the window that takes the mouse to
// rects, in DIPs from the top-left corner of its content: presses, moves
// and the wheel elsewhere go through to what is behind the window, as
// with SetIgnoreMouseEvents, another app's window or the desktop. It is
// what a desktop pet needs, a frameless, transparent window always on
// top: its character and speech bubble take clicks, and the rest of the
// screen works as if the window were not there. Native UI's PassThrough
// only lets the pointer through an element to the window's own content.
//
// nil gives the window all of it back, as it starts; an empty slice none
// of it. Rectangles without area are dropped. Set the region again as
// what takes clicks moves or changes size.
//
// On Linux, the window's input shape is the region (X11's SHAPE
// extension, Wayland's input region). macOS and Windows shape no window's
// input: the window takes the mouse or lets all of it through, switched
// as the pointer comes into the region or leaves it, which macOS tells as
// it moves and Windows is asked at each tick of its clock (15.6 ms) while
// the window shows.
// A button held keeps it as it is, so that a drag that started on the
// character, as one moving the window, ends there; the release settles
// it. In browsers it does nothing.
func (w *Window) SetInputRegion(rects []Rectangle) {
	var region []platform.Rect
	if rects != nil {
		region = make([]platform.Rect, 0, len(rects))
		for _, r := range rects {
			if r.Width > 0 && r.Height > 0 {
				region = append(region, platform.Rect(r))
			}
		}
	}
	w.do(func(n platform.Window) {
		if n, ok := n.(platform.InputRegionWindow); ok {
			n.SetInputRegion(region)
		}
	})
}

// SetActiveAppearance keeps the window drawn as active, the key window of
// the app in front, while it is not (macOS): its controls and material keep
// their active look, as the clear Liquid Glass (VibrancyGlassClear) stays
// clear in a panel floating over another app, which never is active and
// whose glass the system would frost. It uses AppKit's private
// _setForceMainAppearance:, which a later macOS may drop: the window then
// looks as any other, but for its NSVisualEffectView material. Apps for
// the Mac App Store should not call it. On every platform, its pages and
// native UI stay active too (ui.Context.WindowActive).
func (w *Window) SetActiveAppearance(v bool) {
	w.do(func(n platform.Window) {
		n.SetActiveAppearance(v)
		w.keepActive = v
		scheduleActive()
	})
}

// SetWindowButtonsTheme draws the window controls of a window with a
// hidden title bar (TitleBarStyle) for a light or a dark background,
// whatever the app's appearance, for a page whose title bar differs from
// it, as a dark view under the controls of a light app: ThemeDark draws
// light glyphs, ThemeLight dark ones, and ThemeSystem follows the app's
// appearance again (Theme.SetSource). The page's prefers-color-scheme and
// the menus keep the app's. Windows only: the traffic lights of macOS and
// GTK's title buttons on Linux keep the app's appearance.
func (w *Window) SetWindowButtonsTheme(s ThemeSource) {
	if s == "" {
		s = ThemeSystem
	}
	w.do(func(n platform.Window) { n.SetWindowButtonsTheme(string(s)) })
}

// SetContentProtection keeps the window content out of screenshots and
// screen recordings.
func (w *Window) SetContentProtection(v bool) {
	w.do(func(n platform.Window) { n.SetContentProtection(v) })
}

// SetVibrancy sets the material behind the page; VibrancyNone removes it.
// On Windows, it changes the material of a window created with one, and
// shows none in other windows. See WindowOptions.Vibrancy.
func (w *Window) SetVibrancy(v Vibrancy) {
	w.do(func(n platform.Window) { n.SetVibrancy(string(v)) })
	// Native UI looks again whether the material shows.
	w.Invalidate()
}

// SetVibrancyTint tints the window's material, in the syntax of
// BackgroundColor, or "" to remove the tint. Only macOS 26's Liquid Glass
// takes one; other materials and platforms ignore it. See
// WindowOptions.VibrancyTint.
func (w *Window) SetVibrancyTint(color string) error {
	tint, err := backgroundOption(color)
	if err != nil {
		return err
	}
	w.do(func(n platform.Window) {
		w.vibrancyTint = tint
		n.SetVibrancyTint(w.vibrancyTintColor())
	})
	return nil
}

// ProgressState is the state of a progress bar; see ProgressBar.
type ProgressState string

// Progress states.
const (
	ProgressNone          ProgressState = ""
	ProgressNormal        ProgressState = "normal"
	ProgressIndeterminate ProgressState = "indeterminate"
	// ProgressPaused and ProgressError color the bar yellow and red on
	// Windows, and show it like ProgressNormal elsewhere.
	ProgressPaused ProgressState = "paused"
	ProgressError  ProgressState = "error"
)

// ProgressBar is the progress of a task, shown on the window's taskbar
// button (Windows), on the application's Dock icon (macOS) or on its
// launcher entry (Linux docks that implement the Unity launcher API, such
// as KDE Plasma's and Ubuntu's). The zero value shows no progress.
type ProgressBar struct {
	// State defaults to ProgressNormal when Value is above 0, else to
	// ProgressNone.
	State ProgressState
	// Value is the fraction done, between 0 and 1.
	Value float64
}

// SetProgressBar shows the progress of a task:
//
//	win.SetProgressBar(mygo.ProgressBar{Value: done / total})
//	win.SetProgressBar(mygo.ProgressBar{}) // done: remove it
//
// On macOS and Linux the progress belongs to the application: the window
// that set it last wins.
func (w *Window) SetProgressBar(p ProgressBar) {
	state, value := p.State, min(max(p.Value, 0), 1)
	if state == ProgressNone && p.Value > 0 {
		state = ProgressNormal
	}
	w.do(func(n platform.Window) { n.SetProgressBar(string(state), value) })
}

// FlashFrame draws the user's attention to the window, or stops doing so:
// its taskbar button flashes until it is focused (Windows), it is marked
// urgent (Linux), or the Dock icon bounces (macOS, while the app is not
// active).
func (w *Window) FlashFrame(flash bool) {
	w.do(func(n platform.Window) { n.FlashFrame(flash) })
}

// SetSkipTaskbar hides the window from the taskbar, or shows it there again
// (Linux, Windows). See WindowOptions.SkipTaskbar.
func (w *Window) SetSkipTaskbar(v bool) {
	w.do(func(n platform.Window) { n.SetSkipTaskbar(v) })
}

// SetVisibleOnAllWorkspaces shows the window on every workspace (macOS
// Spaces, Linux virtual desktops), or on the current one only. On macOS it
// also shows over other apps in full screen, even with DisableFullScreen,
// if the app is an accessory (ActivationPolicyAccessory, Dock.Hide): macOS
// shows no windows of regular apps there.
func (w *Window) SetVisibleOnAllWorkspaces(v bool) {
	w.do(func(n platform.Window) { n.SetVisibleOnAllWorkspaces(v) })
}

// SetIcon sets the icon of the window, shown in its title bar and taskbar
// button (Linux, Windows), from a PNG image; nil restores the application
// icon. macOS windows show no icon of their own.
func (w *Window) SetIcon(png []byte) error {
	var err error
	w.do(func(n platform.Window) { err = n.SetIcon(png) })
	return err
}

// ThumbButton is a button of the window's thumbnail toolbar, the row of
// small buttons Windows shows under a taskbar button's live preview. See
// SetThumbBar.
type ThumbButton struct {
	// Icon is a PNG image for the button. On updates, nil keeps the
	// button's existing icon.
	Icon []byte
	// ToolTip names the button for the user.
	ToolTip string
	// Disabled grays the button out; Hidden keeps it out of the toolbar.
	Disabled bool
	Hidden   bool
	// Click is called on the main thread when the user clicks the button.
	Click func(b *ThumbButton, win *Window)
}

// SetThumbBar sets the window's thumbnail toolbar (Windows):
//
//	win.SetThumbBar([]mygo.ThumbButton{
//	    {Icon: back, ToolTip: "Back", Click: goBack},
//	    {Icon: play, ToolTip: "Play", Click: play},
//	})
//
// The first call fixes the buttons' count and order; later calls update
// their icons (passing new PNGs, or nil to keep them),
// tooltips, Disabled and Hidden state, and replace their Click functions.
// The toolbar takes one to 7 buttons, and a later call that changes their
// number fails with an error. A button's Click runs on the main thread.
// Elsewhere SetThumbBar returns errors.ErrUnsupported.
func (w *Window) SetThumbBar(buttons []ThumbButton) error {
	if len(buttons) == 0 {
		return errors.New("mygo: a thumbnail toolbar takes one or more buttons")
	}
	if len(buttons) > 7 {
		return errors.New("mygo: a thumbnail toolbar takes at most 7 buttons")
	}
	var err error
	w.do(func(n platform.Window) {
		tb, ok := n.(platform.ThumbBarWindow)
		if !ok {
			err = platform.ErrUnsupported
			return
		}
		// Link the backend hook on the main thread: WndProc and applyTaskbar
		// read these vars on that thread.
		linkThumbBars()
		updating := len(w.thumbBar) != 0
		if updating && len(buttons) != len(w.thumbBar) {
			err = errors.New("mygo: the buttons of a thumbnail toolbar cannot change once they are set")
			return
		}
		// The shell reports a click by the button's position.
		updated := make([]ThumbButton, len(buttons))
		native := make([]platform.ThumbButton, len(buttons))
		for i, b := range buttons {
			switch {
			case len(b.Icon) > 0:
				b.Icon = slices.Clone(b.Icon)
			case updating:
				b.Icon = w.thumbBar[i].Icon
			default:
				err = errors.New("mygo: a thumbnail toolbar button needs an icon")
				return
			}
			updated[i] = b
			native[i] = platform.ThumbButton{Icon: b.Icon, ToolTip: b.ToolTip,
				Disabled: b.Disabled, Hidden: b.Hidden}
		}
		if err = tb.SetThumbBar(native); err == nil {
			w.thumbBar = updated
		}
	})
	return err
}

// SetMenu sets the menu bar of this window (Linux, Windows). On macOS the
// menu bar belongs to the application; see Application.SetMenu.
func (w *Window) SetMenu(m *Menu) {
	onMain(func() {
		w.menu = m
		if w.native != nil {
			w.native.SetMenu(m.snapshot())
		}
	})
}

// SetAutoHideMenuBar keeps the menu bar of this window out of sight until
// the user presses Alt or F10, or shows it for good again (Linux,
// Windows). See WindowOptions.AutoHideMenuBar.
func (w *Window) SetAutoHideMenuBar(v bool) {
	w.do(func(n platform.Window) { n.SetAutoHideMenu(v) })
}

// NativeHandle returns the native window: NSWindow* on macOS, GtkWindow* on
// Linux and HWND on Windows.
func (w *Window) NativeHandle() uintptr { return get(w, platform.Window.Handle) }

// CapturePage returns a PNG screenshot of the window's rendered native UI.
func (w *Window) CapturePage() ([]byte, error) {
	if w.IsDestroyed() {
		return nil, errDestroyed
	}
	if !w.content {
		return nil, errNoContent
	}
	return w.captureContent()
}

var (
	errDestroyed = errors.New("mygo: window has been destroyed")
	errNoContent = errors.New("mygo: the window has no Content")
)

// OnClose is called when the window is about to close. Call
// e.PreventDefault to keep it open, except when the OS ends the session
// (see Application.OnBeforeQuit): the window closes then all the same.
func (w *Window) OnClose(fn func(e *CloseEvent)) (off func()) { return w.onClose.add(fn, false) }

// OnClosed is called after the window has been closed.
func (w *Window) OnClosed(fn func()) (off func()) { return w.onClosed.add(fn, false) }

// OnFocus is called when the window gains focus.
func (w *Window) OnFocus(fn func()) (off func()) { return w.onFocus.add(fn, false) }

// OnBlur is called when the window loses focus.
func (w *Window) OnBlur(fn func()) (off func()) { return w.onBlur.add(fn, false) }

// OnShow is called when the window is shown.
func (w *Window) OnShow(fn func()) (off func()) { return w.onShow.add(fn, false) }

// OnHide is called when the window is hidden.
func (w *Window) OnHide(fn func()) (off func()) { return w.onHide.add(fn, false) }

// OnReadyToShow is called once, when the first page is ready to be
// displayed. Create the window with Hidden and call Show here to avoid a
// visual flash. A page that failed to load makes the window ready too,
// unless an OnDidFailLoad listener loaded another, which the window then
// waits for. A window showing Content is ready once it drew its first
// frame, which a hidden window draws while hidden.
func (w *Window) OnReadyToShow(fn func()) (off func()) { return w.onReadyToShow.add(fn, false) }

// OnResize is called after the window was resized.
func (w *Window) OnResize(fn func()) (off func()) { return w.onResize.add(fn, false) }

// OnFileDrop is called when files, for example from Finder or Explorer, are
// dropped on the page, with their paths. The app's own pages also get them,
// through onFileDrop of mygo-runtime. In a window showing native UI, it
// gets the files no element takes (ui.Element.DroppedFiles), with X and Y
// in DIPs.
//
// The page's own drag and drop keeps working: drop events still reach it
// with the File objects, and drags that start in the page are left alone.
// Only where the page does not handle dragged files are they accepted, so
// that dropping them there reaches OnFileDrop instead of replacing the page
// with the file.
func (w *Window) OnFileDrop(fn func(e *FileDropEvent)) (off func()) {
	return w.onFileDrop.add(fn, false)
}

// OnMove is called after the window was moved.
func (w *Window) OnMove(fn func()) (off func()) { return w.onMove.add(fn, false) }

// OnMaximize is called when the window is maximized.
func (w *Window) OnMaximize(fn func()) (off func()) { return w.onMaximize.add(fn, false) }

// OnUnmaximize is called when the window leaves the maximized state.
func (w *Window) OnUnmaximize(fn func()) (off func()) { return w.onUnmaximize.add(fn, false) }

// OnMinimize is called when the window is minimized.
func (w *Window) OnMinimize(fn func()) (off func()) { return w.onMinimize.add(fn, false) }

// OnRestore is called when the window is restored from minimized.
func (w *Window) OnRestore(fn func()) (off func()) { return w.onRestore.add(fn, false) }

// OnEnterFullScreen is called when the window entered full screen.
func (w *Window) OnEnterFullScreen(fn func()) (off func()) {
	return w.onEnterFullScreen.add(fn, false)
}

// OnLeaveFullScreen is called when the window left full screen.
func (w *Window) OnLeaveFullScreen(fn func()) (off func()) {
	return w.onLeaveFullScreen.add(fn, false)
}

func (w *Window) readyToShow() {
	if w.readyShow {
		return
	}
	w.readyShow = true
	fire(&w.onReadyToShow)
}

// windowHandler receives native window events.
type windowHandler struct{ w *Window }

func (h *windowHandler) ShouldClose() bool {
	e := &CloseEvent{Window: h.w}
	fire1(&h.w.onClose, e)
	if !e.prevented {
		h.w.closeState()
	}
	return !e.prevented
}

func (h *windowHandler) Closed() {
	w := h.w
	if w.native == nil {
		return
	}
	w.cancelDataDrag()
	w.detachContent()
	w.native = nil
	w.destroyed.Store(true)

	windows.Lock()
	for i, x := range windows.list {
		if x == w {
			windows.list = append(windows.list[:i:i], windows.list[i+1:]...)
			break
		}
	}
	if windows.focused == w {
		windows.focused = nil
		scheduleActive()
	}
	children := []*Window{}
	for _, x := range windows.list {
		if x.parent == w {
			children = append(children, x)
		}
	}
	windows.Unlock()

	// Children close with their parent, which reports the last window
	// once they are all gone.
	closingParents++
	for _, c := range children {
		c.destroy()
	}
	closingParents--

	fire(&w.onClosed)
	if closingParents == 0 && len(Windows()) == 0 {
		App.lastWindowClosed()
	}
}

// closingParents counts the windows closing their children (main thread
// only).
var closingParents int

func (h *windowHandler) Focused() {
	backgroundShow = false
	windows.Lock()
	windows.focused = h.w
	windows.Unlock()
	focusCount++
	h.w.focusKnown, h.w.focusedAt = true, focusCount
	scheduleActive()
	fire(&h.w.onFocus)
}

func (h *windowHandler) Blurred() {
	windows.Lock()
	if windows.focused == h.w {
		windows.focused = nil
	}
	windows.Unlock()
	scheduleActive()
	fire(&h.w.onBlur)
}

func (h *windowHandler) MenuItemClicked(id int) {
	if h.w.native != nil {
		menuItemClicked(id, h.w)
	}
}

// ThumbButtonClicked runs the Click of the thumbnail toolbar button at id,
// the position SetThumbBar gave it. The shell only reports clicks of
// buttons that are shown and enabled, and the toolbar's size keeps the
// slice's, so the check is for the cross-layer id alone.
func (h *windowHandler) ThumbButtonClicked(id int) {
	if h.w.native != nil && id >= 0 && id < len(h.w.thumbBar) {
		if b := &h.w.thumbBar[id]; b.Click != nil {
			b.Click(b, h.w)
		}
	}
}

func (h *windowHandler) Resized()           { h.w.stateChanged(); h.w.placeFlyouts(); fire(&h.w.onResize) }
func (h *windowHandler) Moved()             { h.w.stateChanged(); h.w.placeFlyouts(); fire(&h.w.onMove) }
func (h *windowHandler) Minimized()         { h.w.stateChanged(); fire(&h.w.onMinimize) }
func (h *windowHandler) Restored()          { h.w.stateChanged(); fire(&h.w.onRestore) }
func (h *windowHandler) Maximized()         { h.w.stateChanged(); fire(&h.w.onMaximize) }
func (h *windowHandler) Unmaximized()       { h.w.stateChanged(); fire(&h.w.onUnmaximize) }
func (h *windowHandler) EnteredFullScreen() { h.w.stateChanged(); fire(&h.w.onEnterFullScreen) }
func (h *windowHandler) LeftFullScreen()    { h.w.stateChanged(); fire(&h.w.onLeaveFullScreen) }
func (h *windowHandler) TitleBarChanged() {
	if c := h.w.conn; c != nil && c.TitleBarChanged != nil {
		c.TitleBarChanged()
	}
}

// filesDropped tells OnFileDrop listeners about dropped files.
func (w *Window) filesDropped(paths []string, x, y int) {
	if len(paths) == 0 {
		return
	}
	fire1(&w.onFileDrop, &FileDropEvent{Paths: paths, X: x, Y: y})
}
