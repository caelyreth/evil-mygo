// Package platform defines the contract between the public mygo API and the
// native backends (AppKit on macOS, GTK on Linux, Win32 on Windows).
//
// Threading rules: unless a method says otherwise it must be called on the
// main (UI) thread, and every handler callback is invoked on the main thread.
// The public package is responsible for hopping onto the main thread.
package platform

import (
	"errors"
	"time"

	"github.com/egoist/mygo/transfer"
)

// ErrUnsupported is returned by features the current backend cannot provide.
// It is errors.ErrUnsupported too (errors.Is), which apps check.
var ErrUnsupported error = unsupportedError{}

// Unsupported returns ErrUnsupported in a backend's own words, a browser's
// or a phone's: Error reports message, and errors.Is matches it as
// ErrUnsupported and errors.ErrUnsupported.
func Unsupported(message string) error { return unsupportedMessage(message) }

type unsupportedError struct{}

func (unsupportedError) Error() string        { return "not supported on this platform" }
func (unsupportedError) Is(target error) bool { return target == errors.ErrUnsupported }

type unsupportedMessage string

func (e unsupportedMessage) Error() string { return string(e) }
func (unsupportedMessage) Unwrap() error   { return ErrUnsupported }

// ErrNotificationsDenied is what ShowNotification fails with when the user
// does not allow the app to show notifications.
var ErrNotificationsDenied = errors.New("mygo: the user does not allow notifications")

// Point is a position in screen coordinates (DIPs, origin top-left of the
// primary display).
type Point struct{ X, Y int }

// Size is a width and height in DIPs.
type Size struct{ Width, Height int }

// Rect is a rectangle in screen coordinates (DIPs).
type Rect struct{ X, Y, Width, Height int }

// TitleBar is the room the window controls take at the top of a window
// with a hidden title bar, in DIPs: they sit in a band of Height along the
// top edge, Left wide from the left edge and Right wide from the right,
// and end Bottom below the top edge, which is Height where they fill the
// band.
type TitleBar struct{ Height, Left, Right, Bottom int }

// Color is an 8-bit RGBA color.
type Color struct{ R, G, B, A uint8 }

// Backend is implemented by every native backend.
type Backend interface {
	// Name identifies the backend, for example "darwin/wkwebview".
	Name() string

	// Init prepares the native application. It is called once, on the main
	// thread, before Run.
	Init(h AppHandler, opts AppOptions) error
	// Run runs the native event loop until Quit is called. h.Ready is
	// invoked once the application has finished launching.
	Run() error
	// Quit makes Run return.
	Quit()

	// IsMainThread reports whether the caller runs on the UI thread. Safe
	// to call from any goroutine.
	IsMainThread() bool
	// Signal asks the backend to invoke AppHandler.Dispatch on the main
	// thread as soon as possible. Safe to call from any goroutine.
	Signal()
	// Step blocks until at least one native event has been processed or
	// Wake has been called. Main thread only; used for nested waits.
	Step()
	// Wake unblocks a pending Step. Safe to call from any goroutine.
	Wake()

	NewWindow(opts *WindowOptions, h WindowHandler) (Window, error)

	SetApplicationMenu(m *Menu)
	// PopupMenu shows a context menu. pos is relative to the window's
	// content area; nil means the current mouse location. It blocks until
	// the menu is dismissed.
	PopupMenu(m *Menu, w Window, pos *Point)
	// UpdateMenuItem refreshes label/state of every native item created
	// for item.ID.
	UpdateMenuItem(item *MenuItem)

	App() AppController
	Dialogs() Dialogs
	Clipboard() Clipboard
	Shell() Shell
	Screen() Screen
	Theme() Theme
	Power() Power
	Network() Network
	// Secrets may be called before Init, from any goroutine.
	Secrets() Secrets
	// Applications lists the applications installed on the computer.
	// Unlike other methods it may be called from any goroutine once Init
	// returned: it reads the disk, which the core does off the main thread.
	Applications() ([]Application, error)

	// NewFileWatch starts watching files, with no target yet. notify is
	// called on a goroutine of the backend, one notice at a time.
	// NewFileWatch may be called before Init, from any goroutine.
	NewFileWatch(notify func(FileWatchNotice)) (FileWatch, error)
	// ReplaceFile replaces the file at target with the file at
	// replacement, in the same directory, keeping what the system keeps of
	// the target's metadata (permissions, owner, extended attributes,
	// creation date); replacement is gone afterwards. It may be called
	// before Init, from any goroutine.
	ReplaceFile(target, replacement string) error

	// NewTray makes a tray icon. id is TrayOptions.ID, which Linux needs
	// as the icon is made; the other backends take it through Tray.SetID.
	NewTray(id string, h TrayHandler) (Tray, error)
	// RegisterHotkey makes accelerator a global shortcut. description says
	// what it does, for desktops that show the app's shortcuts to the
	// user; such desktops tell the keys they bound with HotkeyBound.
	RegisterHotkey(id int, accelerator, description string) error
	UnregisterHotkey(id int)

	NotificationsSupported() bool
	// ShowNotification shows n; done runs on the main thread once the
	// system has it, or with why it could not.
	ShowNotification(n *Notification, done func(error))
	RemoveNotification(id string)
	// RemoveAllNotifications removes the app's notifications, on macOS
	// those of earlier runs too.
	RemoveAllNotifications()
}

// WindowStateWindow is a window whose bounds in DIPs change with the
// scales of the displays where they differ (Windows).
// WindowStateBounds is its rectangle in the screen's pixels, which window
// states keep with them.
type WindowStateWindow interface {
	WindowStateBounds() Rect
}

// InputRegionWindow is a window that takes the mouse only where its input
// region is (macOS, Linux, Windows): SetInputRegion limits it to region,
// in DIPs from the top-left corner of the content, and lets the mouse
// through elsewhere; nil gives it all of the window, an empty region
// none. SetIgnoreMouseEvents(true) wins over it. Apart from Window, so
// that only apps setting a region link it.
type InputRegionWindow interface {
	SetInputRegion(region []Rect)
}

// ThumbButton is a button of a thumbnail toolbar (ThumbBarWindow): the core
// resolves the buttons of the public SetThumbBar into these, in the order
// the shell reports a click by (the button's index). Icon is always the
// button's PNG; the backend compares it with the one it has.
type ThumbButton struct {
	Icon     []byte
	ToolTip  string
	Disabled bool
	Hidden   bool
}

// ThumbBarWindow is a window with a thumbnail toolbar, the row of small
// buttons Windows shows under a taskbar button's live preview
// (SetThumbBar). Apart from Window, so that only apps setting one link it.
type ThumbBarWindow interface {
	SetThumbBar(buttons []ThumbButton) error
}

// WindowStateBackend restores windows at their pixels (Windows):
// RestoreWindowState sets opts.StateBounds and returns true when pixels,
// zero for none, would show on a connected display. Otherwise the core
// restores the state's DIPs where every display has the same scale, and
// centers the window where they have not.
type WindowStateBackend interface {
	RestoreWindowState(opts *WindowOptions, pixels Rect) bool
}

// StyleBackend styles the widgets the toolkit draws for the app with the
// app's own style sheet (Linux: GTK's CSS, Theme.SetGTKCSS). SetStyleSheet
// replaces the sheet set before, or removes it for "", and keeps that one
// when css does not parse, returning why.
type StyleBackend interface {
	SetStyleSheet(css string) error
}

// WindowButtonsBackend reports the window buttons the desktop puts at the
// start and at the end of title bars (Linux): "minimize", "maximize" and
// "close", in their order, none where the desktop wants none. Main thread
// only.
type WindowButtonsBackend interface {
	WindowButtons() (start, end []string)
}

// ExitBackend releases what the native application must not take into the
// process's exit (Windows: its windows, whose window procedure is Go's and
// cannot run once the process exits). The app calls Exiting once, on the
// main thread, when it has finished and is about to exit; nothing native
// is used after it.
type ExitBackend interface {
	Exiting()
}

// AppOptions configures Backend.Init.
type AppOptions struct {
	Name string
	// ActivationPolicy is "regular" (default), "accessory" or "prohibited".
	// Only meaningful on macOS.
	ActivationPolicy string
	// Background starts the app without bringing it to the front, as
	// `mygo dev -background` asks: on macOS, where an executable that is
	// no bundle is brought there as it starts, it does not activate, and
	// on macOS and Windows ShowInactive orders a window in behind the
	// active app's windows, rather than over them, while another app is
	// the active one. The core shows the windows without the focus
	// everywhere.
	Background bool
}

// AppHandler receives application level events from the backend.
type AppHandler interface {
	Ready()
	// Dispatch drains the queue of functions scheduled for the main thread.
	Dispatch()
	// QuitRequested is called when the user or the OS asks the application
	// to quit (Cmd+Q, Dock > Quit, logout). Returning true lets the quit
	// proceed.
	QuitRequested() bool
	// Terminating is called right before the process is terminated by the
	// OS (logout/shutdown); Run will not return. The core runs the quit
	// listeners then, unless a quit ran them, and finishes the app.
	Terminating()
	// Activated is called when the application is re-activated, e.g. by
	// clicking its Dock icon.
	Activated(hasVisibleWindows bool)
	DidBecomeActive()
	DidResignActive()
	OpenURLs(urls []string)
	OpenFiles(paths []string)
	MenuItemClicked(id int)
	ThemeChanged()
	DisplaysChanged()
	// LocaleChanged is called when the languages the user prefers, or the
	// regional formats, may have changed, on the systems that report it.
	LocaleChanged()
	// PowerEvent is "suspend", "resume", "lock-screen" or "unlock-screen".
	PowerEvent(event string)
	// NetworkChanged reports the network's status, once Network.Watch was
	// called, when it may have changed.
	NetworkChanged(status NetworkStatus)
	HotkeyPressed(id int)
	// HotkeyBound reports the keys a desktop that binds global shortcuts
	// for apps has for one, as it writes them, "" when it bound none.
	HotkeyBound(id int, keys string)
	NotificationClicked(id string)
	// MemoryWarning reports that the system is short of memory, on the
	// systems that say: phones end apps that keep too much.
	MemoryWarning()
}

// AppController exposes application level native features.
type AppController interface {
	SetActivationPolicy(policy string)
	Activate()
	// AllowForeground lets the next process to ask bring its windows to
	// the front, which Windows only lets the process in front do.
	AllowForeground()
	// WithActivationToken runs fn with the startup notification or XDG
	// activation token of the launch that caused it, which the window fn
	// shows or focuses first may need to take the focus (Wayland).
	WithActivationToken(token string, fn func())
	Hide()
	Unhide()
	IsHidden() bool
	// IsActive reports whether the app is the active one: in front on
	// macOS, with a window that has the keyboard elsewhere.
	IsActive() bool
	SetBadge(label string)
	Badge() string
	// Bounce requests user attention and returns an id for CancelBounce.
	Bounce(critical bool) int
	CancelBounce(id int)
	SetDockIcon(png []byte) error
	// SetDockMenu sets the menu of the Dock icon (macOS); nil removes it.
	SetDockMenu(m *Menu)
	ShowAboutPanel(opts AboutPanelOptions)
	// PreferredLanguages returns the languages the user reads, most
	// preferred first, as the system's settings list them, in the forms
	// the system writes them (the core normalizes them): never what the
	// app chose for itself (SaveLocale). Safe before Init.
	PreferredLanguages() []string
	// Region returns the user's regional formats. Safe before Init.
	Region() Region
	// SavedLocale returns the language the app chose for itself, as the
	// system keeps it, and whether the system keeps one at all (macOS:
	// AppleLanguages in the app's own defaults, which System Settings
	// shows as the app's language; browsers: localStorage). Where it
	// keeps none, the core keeps it in a file. Safe before Init.
	SavedLocale() (tag string, kept bool)
	// SaveLocale keeps tag where SavedLocale reads it, "" to forget it,
	// and reports whether the system keeps one at all.
	SaveLocale(tag string) bool
	// SetLanguages tells the backend the languages the app shows, its
	// own language first, and whether it is laid out right to left, for
	// the system's own texts and widgets.
	SetLanguages(tags []string, rtl bool)
	// RegisterURLScheme makes the app, identified by id (its identifier)
	// and named name, the handler of the URLs of scheme for the user.
	RegisterURLScheme(scheme, id, name string) error
	UnregisterURLScheme(scheme, id, name string) error
	IsURLSchemeRegistered(scheme, id, name string) bool
	// SetOpenAtLogin makes the app start at login, passing arg where the
	// start command takes arguments; OpenedAtLogin reports whether the
	// system started it so without one (macOS login items).
	SetOpenAtLogin(open bool, id, name, arg string) error
	OpenAtLogin(id, name, arg string) bool
	OpenedAtLogin() bool
	// Package describes the application bundle the process runs from; ok
	// is false for a plain executable (e.g. `go run`). Safe to call from
	// any goroutine, before Init too.
	Package() (info PackageInfo, ok bool)
	// AddRecentDocument records the absolute path of a document the user
	// opened in the system's recent documents; ClearRecentDocuments
	// removes the ones the app recorded.
	AddRecentDocument(path string) error
	ClearRecentDocuments() error
	// SetJumpList commits the app's whole jump list (Windows); elsewhere
	// it does nothing.
	SetJumpList(list JumpList) error
}

// Region is the user's regional formats, as the system's settings have
// them with the user's changes.
type Region struct {
	// Locale is the locale of the formats, in the form the system writes
	// it, "" when unknown.
	Locale string
	// FirstWeekday is the first day of the week, 0 for Sunday to 6 for
	// Saturday, or -1 when the system does not say.
	FirstWeekday int
	// HourCycle is 12 or 24, or 0 when the system does not say.
	HourCycle int
}

// PackageInfo is metadata of a packaged application.
type PackageInfo struct {
	Name       string
	Version    string
	Identifier string
}

// AboutPanelOptions configures the standard about panel.
type AboutPanelOptions struct {
	ApplicationName    string
	ApplicationVersion string
	Version            string
	Copyright          string
	Credits            string
}

// JumpList is the whole jump list of the app: Windows replaces all of it
// at each commit.
type JumpList struct {
	Tasks []JumpListTask
}

// JumpListTask is a task of a jump list: a program to run with its
// arguments, unquoted, its label, tooltip and icon.
type JumpListTask struct {
	Title       string
	Description string
	Path        string
	Args        []string
	WorkingDir  string
	IconPath    string
	IconIndex   int
}

// WindowOptions configures a native window and its webview. The public
// package fills in all defaults before calling NewWindow.
type WindowOptions struct {
	Title          string
	X, Y           int
	Width, Height  int
	Center         bool
	UseContentSize bool
	MinSize        Size
	MaxSize        Size
	// StateBounds is the window's restored normal rectangle in the
	// screen's pixels, set by WindowStateBackend. It overrides X, Y,
	// Width and Height.
	StateBounds *Rect

	Resizable      bool
	Movable        bool
	Minimizable    bool
	Maximizable    bool
	Closable       bool
	Focusable      bool
	Fullscreenable bool
	AlwaysOnTop    bool
	FullScreen     bool
	Maximized      bool
	SkipTaskbar    bool
	AutoHideMenu   bool
	HasShadow      bool
	Frameless      bool
	Transparent    bool
	// ConsumeActivationClick withholds the activation press from a
	// window's content (macOS, Windows). Other platforms ignore it.
	ConsumeActivationClick bool
	// BackgroundColor is painted before the page renders; nil keeps the
	// platform default.
	BackgroundColor *Color
	// TitleBarStyle is "default", "hidden" or "hiddenInset". A hidden title
	// bar gives the page the whole window, under the window controls:
	// AppKit's traffic lights on macOS, and on Linux and Windows controls
	// the backend puts over the page (see Window.TitleBar).
	TitleBarStyle string
	// TitleBarHeight is the height of a hidden title bar in DIPs (Linux,
	// Windows); 0 is the backend's own.
	TitleBarHeight int
	// HideWindowButtons hides the window controls of a hidden title bar,
	// which the page draws itself: TitleBar is then zero.
	HideWindowButtons    bool
	TrafficLightPosition *Point
	Vibrancy             string
	// VibrancyTint tints the material, where the platform's material takes
	// one (macOS 26's Liquid Glass): nil for none.
	VibrancyTint *Color
	// CornerRadius rounds a frameless window's corners, in DIPs: its
	// content and material are cut to them (macOS), or the system rounds
	// it with its own corners (Windows 11).
	CornerRadius int
	Opacity      float64
	Parent       Window
	Modal        bool
	// Flyout makes the window a flyout of its Parent, placed next to an
	// anchor (Window.PlaceFlyout), at Width and Height: borderless,
	// transparent, out of the taskbar, with Focusable telling whether it
	// takes the keyboard. Show of a flyout that is not focusable shows it
	// without the keyboard, as ShowInactive.
	Flyout *Flyout
	// Panel makes the window a panel (mygo.WindowOptions.Panel): it floats
	// above the windows of all apps on the current workspace, full-screen
	// ones included, out of the taskbar and the app switcher, and on macOS
	// takes the keyboard without activating the app.
	Panel bool

	DevTools bool
	// Surface creates the window with a Surface, which content MyGo draws
	// itself fills: Window.Surface returns it.
	Surface bool
	// OwnScene opens the window as a window of the system's own, on iPad.
	OwnScene bool
}

// Window is a native window.
type Window interface {
	// Handle returns the native window (NSWindow*, GtkWindow*, HWND).
	Handle() uintptr
	// Surface returns the drawing surface of a window created with
	// WindowOptions.Surface, nil for others.
	Surface() Surface

	SetTitle(title string)
	Title() string
	// SetRepresentedFile shows the file a window shows, "" for none: the
	// proxy icon of the title bar and its menu of folders (macOS). Other
	// platforms do nothing.
	SetRepresentedFile(path string)
	// SetDocumentEdited shows that the window's document has unsaved
	// changes: the dot of the close button (macOS). Other platforms do
	// nothing; the core marks the title there.
	SetDocumentEdited(edited bool)
	SetBounds(r Rect)
	Bounds() Rect
	SetContentBounds(r Rect)
	ContentBounds() Rect
	SetMinimumSize(s Size)
	SetMaximumSize(s Size)
	SetResizable(v bool)
	IsResizable() bool
	SetMovable(v bool)
	IsMovable() bool
	SetMinimizable(v bool)
	IsMinimizable() bool
	SetMaximizable(v bool)
	IsMaximizable() bool
	SetClosable(v bool)
	IsClosable() bool
	SetAlwaysOnTop(v bool)
	IsAlwaysOnTop() bool
	Show()
	ShowInactive()
	Hide()
	// IsVisible reports whether Show showed the window and Hide did not
	// hide it since: a minimized window is visible (#1542).
	IsVisible() bool
	Focus()
	Blur()
	IsFocused() bool
	// CornerRadius is the radius, in DIPs, the window's corners are
	// rounded to on screen, or 0 when they are square or the system
	// does not tell it.
	CornerRadius() float64
	Minimize()
	IsMinimized() bool
	Maximize()
	Unmaximize()
	IsMaximized() bool
	Restore()
	SetFullScreen(v bool)
	IsFullScreen() bool
	Center()
	SetBackgroundColor(c Color)
	SetOpacity(v float64)
	Opacity() float64
	SetHasShadow(v bool)
	HasShadow() bool
	SetIgnoreMouseEvents(v bool)
	IgnoresMouseEvents() bool
	SetContentProtection(v bool)
	SetVibrancy(material string)
	// SetVibrancyTint tints the window's material, where it takes one
	// (macOS 26's Liquid Glass); nil removes the tint. Other materials and
	// platforms ignore it.
	SetVibrancyTint(c *Color)
	// SetActiveAppearance keeps the window drawn as active, as the key
	// window of the app in front, while it is not (macOS).
	SetActiveAppearance(v bool)
	// SetWindowButtonsTheme draws the controls of a hidden title bar for a
	// "light" or "dark" background whatever the app's appearance, or
	// follows it again with "system" (Windows).
	SetWindowButtonsTheme(source string)
	// SetProgressBar shows progress on the taskbar button, Dock icon or
	// launcher entry: state is "" (none), "normal", "indeterminate",
	// "paused" or "error", value between 0 and 1.
	SetProgressBar(state string, value float64)
	FlashFrame(flash bool)
	SetSkipTaskbar(v bool)
	SetVisibleOnAllWorkspaces(v bool)
	// SetIcon sets the window icon from a PNG image; nil restores the
	// application's.
	SetIcon(png []byte) error
	// SetMenu sets a per-window menu bar (Linux/Windows). No-op on macOS.
	SetMenu(m *Menu)
	// SetAutoHideMenu hides the menu bar until Alt or F10 brings the
	// keyboard to it (Linux/Windows). No-op on macOS.
	SetAutoHideMenu(v bool)
	// TitleBar returns the room the window controls of a window with a
	// hidden title bar take, which WindowHandler.TitleBarChanged reports
	// changes of. It is zero for other windows and while the controls are
	// hidden, in full screen.
	TitleBar() TitleBar
	// StartDrag moves the window with the mouse (frameless drag regions).
	StartDrag()
	// TitleBarDoubleClicked performs the platform action for a double click
	// on a custom title bar (zoom/minimize on macOS).
	TitleBarDoubleClicked()
	// PlaceFlyout places a flyout (WindowOptions.Flyout) of the given
	// size next to its anchor again, as Flyout.Place resolves it in the
	// work area of a display, after the anchor, the size or the parent
	// changed. It does nothing for other windows.
	PlaceFlyout(f Flyout, size Size)
	// Close destroys the window without asking WindowHandler.ShouldClose.
	Close()
}

// EmbeddedView is an embedded platform or plugin view in a window showing
// a Surface (ViewSurface.EmbedView, PluginViewSurface.NewPluginView), under
// the surface, which shows it where its frames are transparent, as holes
// (scene.OpHole) make them.
type EmbeddedView interface {
	// Focus gives the embedded view the keyboard.
	Focus()
	// TabInto gives the embedded view the keyboard as Tab does.
	TabInto(back bool)
	// Close destroys or detaches the embedded view.
	Close()
}

// ViewPlacement is where the content of a surface shows an embedded view
// (ViewSurface.PlaceViews), in DIPs relative to the surface.
type ViewPlacement struct {
	View EmbeddedView
	// Frame is the view's box as it shows; Clip the part of it that
	// shows, which the frames leave transparent but where the content
	// painted over it.
	Frame, Clip RectF
	// Width and Height are the size the view lays out at, in DIPs. Zero
	// is Frame's size.
	Width, Height float64
	// Radii are those of the view's corners, in DIPs, from its top-left
	// clockwise, as the hole showing it has.
	Radii [4]float64
	// Opacity is how opaque the view shows, 0 to 1.
	Opacity float64
	// Covers are the boxes, within Clip, of what the content painted over
	// the view that takes the pointer.
	Covers []RectF
}

// Size returns the size the view lays out at, in its DIPs.
func (p *ViewPlacement) Size() (w, h float64) {
	if p.Width <= 0 || p.Height <= 0 {
		return p.Frame.W, p.Frame.H
	}
	return p.Width, p.Height
}

// Scale returns how much Frame scales the view: 1, 1 unless a transform
// scales it.
func (p *ViewPlacement) Scale() (x, y float64) {
	w, h := p.Size()
	if w <= 0 || h <= 0 {
		return 1, 1
	}
	return p.Frame.W / w, p.Frame.H / h
}

// ToView converts a point of the surface to the view's DIPs.
func (p *ViewPlacement) ToView(x, y float64) (float64, float64) {
	sx, sy := p.Scale()
	return (x - p.Frame.X) / sx, (y - p.Frame.Y) / sy
}

// Rounded reports whether the view's corners are rounded.
func (p *ViewPlacement) Rounded() bool { return p.Radii != [4]float64{} }

// At reports whether the pointer at x, y hits the embedded view.
func (p *ViewPlacement) At(x, y float64) bool {
	if !p.Clip.Contains(x, y) {
		return false
	}
	for _, c := range p.Covers {
		if c.Contains(x, y) {
			return false
		}
	}
	if p.Rounded() {
		w, h := p.Size()
		px, py := p.ToView(x, y)
		return InsideCorners(w, h, p.Radii, px, py)
	}
	return true
}

// InsideCorners reports whether x, y is inside the corners of radii
// (from the top-left clockwise) of a box w by h at the origin.
func InsideCorners(w, h float64, radii [4]float64, x, y float64) bool {
	// Each corner's center, and the side of it the corner is on.
	for i, c := range [4][2]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
		r := radii[i]
		if r <= 0 {
			continue
		}
		cx, cy := r+c[0]*(w-2*r), r+c[1]*(h-2*r)
		dx, dy := x-cx, y-cy
		if (c[0] == 0) != (dx < 0) || (c[1] == 0) != (dy < 0) {
			continue // not in the corner's square
		}
		if dx*dx+dy*dy > r*r {
			return false
		}
	}
	return true
}

// WindowHandler receives window and surface events from the backend.
type WindowHandler interface {
	// ShouldClose is called when the user tries to close the window.
	ShouldClose() bool
	// Closed is called after the native window has been destroyed.
	Closed()
	Focused()
	Blurred()
	// MenuItemClicked reports an item chosen from this window's menu bar
	// or context menu, even while the menu has focus instead of the window.
	MenuItemClicked(id int)
	Resized()
	Moved()
	Minimized()
	Restored()
	Maximized()
	Unmaximized()
	EnteredFullScreen()
	LeftFullScreen()
	// TitleBarChanged is called when Window.TitleBar changes, e.g. when the
	// window enters full screen or the desktop's button layout changes.
	TitleBarChanged()

	// SurfaceEvent delivers input on the window's Surface, and changes of
	// it. For FileDragOver and FileDrop it reports whether the content
	// takes the files where they are, for KeyPressed whether it took
	// the key itself, which the system then leaves alone (Windows opens no
	// menu for Alt and the key), for TextComposition whether it took the
	// composition, which the input method gives up otherwise (macOS), for
	// PointerScroll whether the content scrolled or a handler took it
	// (with Swipe, whether a handler took it or the content scrolled
	// sideways), and
	// for SurfaceCommandEnabled whether the command would act; it returns
	// false for other events.
	SurfaceEvent(ev SurfaceEvent) bool
}

// MenuItemType is the kind of a menu item.
type MenuItemType int

// Menu item kinds.
const (
	MenuItemNormal MenuItemType = iota
	MenuItemSeparator
	MenuItemSubmenu
	MenuItemCheckbox
	MenuItemRadio
)

// Menu is a snapshot of a menu tree.
type Menu struct {
	Items []*MenuItem
	// Palette shows the menu, a submenu, as a row of its items' Swatch
	// colors in place of the item it is the submenu of, where the backend
	// draws such a row (macOS): elsewhere it is a submenu as any other,
	// its items showing their swatches as their icons.
	Palette bool
	// RightToLeft lays a context menu out right to left, as that of an
	// interface laid out so: its items from the right, opening to the
	// left of where it shows, which is its top right corner.
	RightToLeft bool
}

// BadgeAlignment is where an item's badge sits in the column the badges of
// its menu share.
type BadgeAlignment string

const (
	BadgeAlignLeft   BadgeAlignment = "left"
	BadgeAlignCenter BadgeAlignment = "center"
	BadgeAlignRight  BadgeAlignment = "right"
)

// BadgeShape is the background behind a badge.
type BadgeShape string

const (
	BadgeShapeSquare  BadgeShape = "square"
	BadgeShapeRounded BadgeShape = "rounded"
	BadgeShapePill    BadgeShape = "pill"
)

// MenuItem is a snapshot of a menu item. ID identifies the public item the
// native item was built from.
type MenuItem struct {
	ID             int
	Type           MenuItemType
	Label          string
	Subtitle       string
	Badge          string
	BadgeAlign     BadgeAlignment
	BadgeShape     BadgeShape
	BadgeColor     *Color
	BadgeTextColor *Color
	KeepOpen       bool
	Role           string
	Accelerator    string
	Enabled        bool
	Visible        bool
	Checked        bool
	ToolTip        string
	// Icon is the image beside the label, nil for none.
	Icon *MenuIcon
	// IconColor, when set, is the color of a template Icon in the menus
	// package ui draws, in place of the text's; backends ignore it.
	IconColor *Color
	// Swatch is the color the item stands for in a Palette menu, nil for
	// none.
	Swatch  *Color
	Submenu *Menu
}

// FileFilter restricts the files shown in file dialogs: those of its
// Extensions, and of its MIMETypes, which may be "kind/*".
type FileFilter struct {
	Name       string
	Extensions []string
	MIMETypes  []string
}

// OpenDialogOptions configures an open panel.
type OpenDialogOptions struct {
	Title                      string
	DefaultPath                string
	ButtonLabel                string
	Message                    string
	Filters                    []FileFilter
	OpenFiles                  bool
	OpenDirectories            bool
	Multiple                   bool
	ShowHiddenFiles            bool
	CreateDirectories          bool
	TreatPackagesAsDirectories bool
}

// SaveDialogOptions configures a save panel.
type SaveDialogOptions struct {
	Title                      string
	DefaultPath                string
	ButtonLabel                string
	Message                    string
	NameFieldLabel             string
	Filters                    []FileFilter
	ShowHiddenFiles            bool
	CreateDirectories          bool
	TreatPackagesAsDirectories bool
}

// MessageBoxOptions configures a message box.
type MessageBoxOptions struct {
	// Type is "none", "info", "error", "question" or "warning".
	Type            string
	Title           string
	Message         string
	Detail          string
	Buttons         []string
	DefaultID       int
	CancelID        int
	DestructiveIDs  []int
	CheckboxLabel   string
	CheckboxChecked bool
}

// MessageBoxResult is the outcome of a message box.
type MessageBoxResult struct {
	Response        int
	CheckboxChecked bool
}

// Dialogs shows native dialogs. Callbacks run on the main thread exactly
// once. parent may be nil for application modal dialogs.
type Dialogs interface {
	ShowOpenDialog(parent Window, opts *OpenDialogOptions, cb func(paths []string, err error))
	ShowSaveDialog(parent Window, opts *SaveDialogOptions, cb func(path string, err error))
	ShowMessageBox(parent Window, opts *MessageBoxOptions, cb func(res MessageBoxResult, err error))
}

// Clipboard accesses the system clipboard.
type Clipboard interface {
	// WatchChanges observes clipboard changes without reading data. It does
	// not report the initial contents. changed and the returned stop function
	// run on the main thread; stop prevents later callbacks. Only one watch
	// is active at a time. Unsupported backends return ErrUnsupported.
	WatchChanges(changed func()) (stop func(), err error)
	// Data reads copy accepted representations, so returned data outlives
	// native ownership. Nil formats reads all; a missing explicit match is
	// transfer.ErrFormat. WriteData takes an application-owned snapshot;
	// concealed marks it as confidential for the clipboard managers that
	// read the platform's mark; released runs once when native providers
	// are no longer needed, never after a failed write. Neither operation
	// depends on a source window.
	ReadData(formats []transfer.Format) (transfer.Data, error)
	WriteData(data transfer.Data, concealed bool, released func()) error
	Formats() []transfer.Format
	// Flush materializes owned providers and asks the system to persist the
	// data. Close drops any remaining owned providers at app shutdown.
	Flush() error
	Close()
	ReadText() string
	WriteText(text string)
	ReadHTML() string
	WriteHTML(markup string)
	ReadImage() []byte
	WriteImage(png []byte) error
	Clear()
	AvailableFormats() []string
}

// LaterClipboard is a clipboard whose writes finish later, as a
// browser's, whose clipboard answers with a promise: WriteTextThen puts
// text there as WriteText does, and gives done what came of it, on the
// main thread. Desktops' clipboards answer at once (WriteData).
type LaterClipboard interface {
	WriteTextThen(text string, done func(error))
}

// Shell integrates with the desktop environment.
type Shell interface {
	OpenExternal(url string) error
	OpenPath(path string) error
	ShowItemInFolder(path string)
	TrashItem(path string) error
	// TrashItemResult moves the item at path to the trash and returns the
	// path where it landed inside the trash directory. It is supported on
	// macOS only; all other platforms return ("", ErrUnsupported).
	TrashItemResult(path string) (string, error)
	// PreviewItem shows the item at path in the Quick Look panel, the
	// preview Finder opens with the space bar. It is supported on macOS
	// only; all other platforms return ErrUnsupported.
	PreviewItem(path string) error
	Beep()
	// FileIcon returns the icon the file manager shows for a file, a
	// folder or an application, as a PNG image of size by size pixels.
	FileIcon(path string, size int) ([]byte, error)
	// TypeIcon returns the icon the file manager shows for files with the
	// extension ext (without its dot), as FileIcon does, though no such
	// file exists.
	TypeIcon(ext string, size int) ([]byte, error)
	// OpenApplication starts the application at Application.Path, or
	// brings it to the front.
	OpenApplication(path string) error
	// DefaultApp returns the application the system would use to open
	// url. url may be a web URL such as https://example.com, a file://
	// URL, or any URL whose scheme is registered with the system. It
	// is supported on macOS, Linux and Windows; other platforms return
	// (Application{}, ErrUnsupported).
	DefaultApp(url string) (Application, error)
}

// Application is an installed application (Backend.Applications): Path is
// where FileIcon finds its icon and OpenApplication starts it.
type Application struct {
	Name, ID, Path string
}

// Display describes a monitor.
type Display struct {
	ID          int64
	Label       string
	Bounds      Rect
	WorkArea    Rect
	ScaleFactor float64
	Rotation    int
	Internal    bool
	Primary     bool
}

// Screen queries monitors.
type Screen interface {
	Displays() []Display
	CursorPoint() Point
}

// Theme reads and overrides the system appearance.
type Theme interface {
	IsDark() bool
	// SetSource is "system", "light" or "dark".
	SetSource(source string)
	// UIFont returns the family of the desktop's interface font where the
	// system's text stack does not know it, as on Linux, else "".
	UIFont() string
	// FontRendering returns how the desktop's settings say to rasterize
	// text where the system's text stack does not know them, as on Linux.
	FontRendering() FontRendering
	// Preferences returns the settings of the desktop that the system's
	// controls follow. A change of any of them calls Handler.ThemeChanged.
	Preferences() Preferences
}

// Preferences are settings of the desktop that the system's own controls
// follow, and native UI with them.
type Preferences struct {
	// Accent is the accent color the user chose; A is 0 where the desktop
	// has none.
	Accent Color
	// ReduceMotion asks for less motion: macOS's Reduce Motion, Windows's
	// animation effects and GNOME's animations turned off.
	ReduceMotion bool
	// ReduceTransparency asks for opaque surfaces in place of translucent
	// ones: macOS's Reduce Transparency, Windows's transparency effects
	// turned off, prefers-reduced-transparency in browsers. Linux desktops
	// have no such setting.
	ReduceTransparency bool
	// HighContrast asks for more contrast: macOS's Increase Contrast,
	// Windows's contrast themes, the desktop portal's higher contrast.
	HighContrast bool
	// TextScale is how many times larger than usual text should be, as
	// Windows's and GNOME's text size settings say; 0 or 1 for usual.
	TextScale float64
	// Pointer is how accurate the primary pointing device is, and
	// AnyPointer the most accurate of all, as Media Queries 4's pointer
	// and any-pointer: PointerFine, a mouse's, where it is not known.
	Pointer, AnyPointer PointerAccuracy
	// NoHover tells that the primary pointing device cannot hover, as a
	// touch screen, and NoAnyHover that none can (hover and any-hover:
	// none).
	NoHover, NoAnyHover bool
}

// PointerAccuracy is how accurately a pointing device points.
type PointerAccuracy uint8

const (
	// PointerFine is a mouse's, a touchpad's or a pen's.
	PointerFine PointerAccuracy = iota
	// PointerCoarse is a finger's on a touch screen.
	PointerCoarse
	// PointerNone is that of no pointing device at all.
	PointerNone
)

// FontRendering is how the desktop's settings say to rasterize text, as
// GTK's gtk-xft settings: each field is "" for the default, or unknown.
type FontRendering struct {
	// Antialias is "none", "gray" or "subpixel".
	Antialias string
	// Hinting is "none", "slight", "medium" or "full".
	Hinting string
	// Subpixels is the order of the screen's subpixels: "rgb", "bgr",
	// "vrgb" or "vbgr".
	Subpixels string
}

// Power reports power and session events and keeps the computer awake.
type Power interface {
	// Watch starts reporting AppHandler.PowerEvent.
	Watch()
	// KeepAwake keeps the system, and with display the display, from
	// sleeping while idle until release is called.
	KeepAwake(display bool, reason string) (release func())
	// KeepRunning keeps the system from slowing the app down while none
	// of its windows shows, as macOS's App Nap and Windows' power
	// throttling do, until release is called; it leaves the system free
	// to sleep when idle.
	KeepRunning(reason string) (release func())
	OnBattery() bool
	IdleTime() time.Duration
}

// NetworkStatus is the state of the computer's network connection.
type NetworkStatus struct {
	Online  bool
	Metered bool // expensive, metered or in a low data mode
}

// Network reports the state of the computer's network connection.
type Network interface {
	// Watch starts reporting AppHandler.NetworkChanged, which may also be
	// called when the status stayed the same.
	Watch()
	// Status returns the current state.
	Status() NetworkStatus
}

// ErrSecretNotFound is what Secrets.Secret fails with when the store has
// no secret for the service and account.
var ErrSecretNotFound = errors.New("mygo: secret not found")

// Secrets keeps small secrets in the system's credential store: the
// Keychain on macOS, the Secret Service on Linux (through libsecret) and
// the Credential Manager on Windows. A secret is identified by a service,
// the app, and an account, its key, as Electron's keytar stores them.
//
// Unlike other platform methods these may be called from any goroutine,
// and they may block while the system asks the user, e.g. to unlock the
// keyring: the core never calls them on the main thread of a running app.
type Secrets interface {
	SetSecret(service, account string, secret []byte) error
	// Secret fails with ErrSecretNotFound when there is none.
	Secret(service, account string) ([]byte, error)
	// DeleteSecret succeeds when there is no secret to delete.
	DeleteSecret(service, account string) error
}

// TrayHandler receives tray icon events.
type TrayHandler interface {
	Clicked()
	RightClicked()
	// AppearanceChanged tells that the bar the icon shows on turned dark
	// or light.
	AppearanceChanged()
}

// Tray is a status bar / notification area icon.
type Tray interface {
	// SetID names the icon across launches (macOS's autosave name).
	SetID(id string)
	// Dark reports whether the bar the icon shows on is dark.
	Dark() bool
	SetImage(png []byte, template bool) error
	SetTitle(title string)
	// SetTitleMonospaced draws the title in the system's monospaced font,
	// every character as wide as the others, where the icon has a title
	// (macOS).
	SetTitleMonospaced(on bool)
	SetToolTip(tip string)
	SetMenu(m *Menu)
	PopUpMenu(m *Menu)
	// SetActivatable tells the panel whether the icon answers activation,
	// so that it can tell a single click from a double one. Platforms that
	// report clicks themselves ignore it.
	SetActivatable(on bool)
	Bounds() Rect
	Destroy()
}

// Notification is a desktop notification.
type Notification struct {
	ID       string
	Title    string
	Subtitle string
	Body     string
	Silent   bool
	// Group gathers the notifications that share it (threadIdentifier on
	// macOS).
	Group string
	// Image is shown with the notification: PNG or JPEG, or nil. The core
	// links the backend's code of images (LinkNotificationImages) before
	// it gives one.
	Image  []byte
	Sender *NotificationSender
}

// NotificationSender describes an incoming direct message's sender.
type NotificationSender struct {
	ID, Name string
	Avatar   []byte
}

// DeviceBackend is a Backend of a device held in the hand: phones and
// tablets. Backends of desktops have none, and the core's modules do
// nothing or report that they cannot.
type DeviceBackend interface {
	// Haptic plays a haptic: "selection", "light", "medium", "heavy",
	// "success", "warning" or "error".
	Haptic(kind string)
	// SetSystemBars sets the color of the icons of the system's bars,
	// "auto", "light" or "dark", and whether the bars hide.
	SetSystemBars(style string, hidden bool)
	// SetOrientations limits the orientations the screen may take: a mask
	// of Orientation values, 0 for any.
	SetOrientations(mask uint32)
	// Share shows the system's share sheet; done is called once it is
	// shown, or with why it could not be.
	Share(s ShareRequest, done func(error))
	// SaveFile has the user choose where to keep a copy of the file at path.
	SaveFile(path, name string, done func(saved string, err error))
	// Battery tells the state of the battery.
	Battery() BatteryStatus
	// SetBarItems sets the buttons at the end of w's navigation bar.
	SetBarItems(w Window, items []BarItem)
}

// PluginBackend is a Backend whose host runs plugins' native code: the Swift
// and Kotlin that plugins bring (mygo-plugin.json), which it reaches by name.
type PluginBackend interface {
	// PluginCall calls a method of a plugin's native code, with JSON, and
	// done with its JSON answer, or an error (a PluginError, with its code).
	PluginCall(plugin, method string, args []byte, done func(result []byte, err error))
	// OnPluginEvent sets who hears the events of plugins' native code.
	OnPluginEvent(fn func(plugin, name string, data []byte))
}

// PluginError is an error of a plugin's native code.
type PluginError struct{ Code, Message string }

func (e *PluginError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

// Orientations of the screen, for DeviceBackend.SetOrientations.
const (
	OrientationPortrait uint32 = 1 << iota
	OrientationPortraitUpsideDown
	OrientationLandscapeLeft
	OrientationLandscapeRight
)

// ShareRequest is what a share sheet shares. X and Y are where in the window
// in front it comes from, which an iPad's sheet points at.
type ShareRequest struct {
	Text, URL string
	Paths     []string
	X, Y      float64
}

// BatteryStatus is the state of a device's battery. Level is from 0 to 1,
// negative where it is not known.
type BatteryStatus struct {
	Level    float64
	Charging bool
	LowPower bool
}

// BarItem is a button of a window's navigation bar.
type BarItem struct {
	Title, Symbol string
	// Icon is a PNG drawn as a template (Tab.Icon).
	Icon    []byte
	OnClick func()
}

// NavigationBackend is a Backend with the navigation of phones: a bar of
// tabs, each showing a window, and stacks of windows pushed over others,
// both the system's own (UIKit's tab bar and navigation controllers,
// Android's bottom navigation and toolbars). Backends of desktops have
// none: the core shows the windows as windows.
type NavigationBackend interface {
	// NewTabs shows a bar of tabs at the bottom of the screen, tab i
	// showing tabs[i].Window, over the windows shown before.
	NewTabs(tabs []Tab, h TabsHandler) (TabBar, error)
	// PushWindow shows child over parent in parent's stack, with a bar
	// at the top holding child's title and a back button, which pops it.
	PushWindow(parent, child Window, animated bool)
	// PopWindow takes w out of its stack, back to the window under it.
	PopWindow(w Window, animated bool)
}

// Tab is a tab of a TabBar.
type Tab struct {
	Title string
	// Symbol names an SF Symbol, which iOS draws; Icon is a PNG drawn as
	// a template (its alpha, in the bar's colors), which Android draws,
	// and iOS without a Symbol.
	Symbol string
	Icon   []byte
	Window Window
}

// TabBar is a bar of tabs of a NavigationBackend.
type TabBar interface {
	Select(i int)
	SetBadge(i int, badge string)
	Close()
}

// TabsHandler hears of a TabBar, on the main thread.
type TabsHandler interface {
	// TabSelected reports that the user chose tab i.
	TabSelected(i int)
}

// NavigationHandler is a WindowHandler told that the user popped its
// window off its stack (the back button, a swipe, Android's back).
type NavigationHandler interface {
	Popped()
}

// ThumbBarHandler is a WindowHandler told that the user clicked a button
// of its window's thumbnail toolbar, by its ID (Windows).
type ThumbBarHandler interface {
	ThumbButtonClicked(id int)
}
