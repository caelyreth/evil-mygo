package mobile

import (
	"slices"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// window is a screen of the app: a surface for native UI filling the screen
// while it is the window in front. Its size is the screen's, which the host
// reports (Resized) with the insets of what covers its edges, the system's
// bars and the keyboard among them.
// Main thread only, as the core uses it.
type window struct {
	b    *Backend
	id   int
	h    platform.WindowHandler
	opts platform.WindowOptions
	s    *surface

	title   string
	visible bool
	closed  bool
	focused bool
	bg      platform.Color

	// The size the host reported (Resized), in DIPs, and its insets.
	width, height, scale float64
	insets               platform.Insets
	keyboard             float64
	barItems             []platform.BarItem
	// tabs is the bar of tabs the window is on, in tab tabIndex.
	tabs     *tabBar
	tabIndex int
	// devTools says its developer tools are on, in a development build.
	devTools bool
	// parent is the surface's window this one shows under, as a plugin or
	// embedded view in native UI, and children those it holds.
	parent   *window
	children []*window
}

func (b *Backend) newWindow(opts *platform.WindowOptions, h platform.WindowHandler) (*window, error) {
	return b.makeWindow(opts, h, nil, "", "")
}

// makeWindow makes a window, within parent if any: a surface window, or with a
// viewType a plugin's native view.
func (b *Backend) makeWindow(opts *platform.WindowOptions, h platform.WindowHandler, parent *window, viewType, viewArgs string) (*window, error) {
	b.nextID++
	w := &window{b: b, id: b.nextID, h: h, opts: *opts, title: opts.Title, scale: 1, parent: parent, devTools: opts.DevTools}
	c := wire.CreateWindow{
		ID: w.id, Surface: opts.Surface || viewType == "", OwnScene: opts.OwnScene,
		Inspectable: opts.DevTools, ViewType: viewType, ViewArgs: viewArgs,
	}
	if parent != nil {
		c.Parent = parent.id
		parent.children = append(parent.children, w)
	}
	if opts.BackgroundColor != nil {
		c.Background, c.HasBackground = rgba(*opts.BackgroundColor), true
		w.bg = *opts.BackgroundColor
	}
	var r wire.CreateWindowReply
	b.call(&c, &r)
	if err := errorOf(r.Error); err != nil {
		return nil, err
	}
	if opts.Surface || viewType == "" {
		w.s = newSurface(w)
		w.s.handle = uintptr(r.Layer) // iOS's layer; Android's window comes with SurfaceChanged
	}
	b.windows[w.id] = w
	return w, nil
}

// front is the window on screen, nil for none.
func (b *Backend) front() *window {
	if len(b.shown) == 0 {
		return nil
	}
	return b.shown[len(b.shown)-1]
}

// setFront makes the window in front w (nil for none) and tells the
// windows whose focus that changes.
func (b *Backend) refocus(old *window) {
	now := b.front()
	if old == now {
		return
	}
	if old != nil && old.focused {
		old.focused = false
		old.h.Blurred()
	}
	if now != nil && !now.focused {
		now.focused = true
		now.h.Focused()
	}
}

func (w *window) Handle() uintptr { return uintptr(w.id) }

// ViewID is the number the host knows the window by, which a plugin's native
// code knows its views by.
func (w *window) ViewID() int { return w.id }

func (w *window) Surface() platform.Surface {
	if w.s == nil {
		return nil
	}
	return w.s
}

func (w *window) SetTitle(t string) {
	if t != w.title {
		w.title = t
		w.b.call(&wire.SetTitle{ID: w.id, Title: t}, nil)
	}
}

func (w *window) Title() string                              { return w.title }
func (w *window) SetRepresentedFile(string)                  {}
func (w *window) SetDocumentEdited(bool)                     {}
func (w *window) SetBounds(platform.Rect)                    {}
func (w *window) SetContentBounds(platform.Rect)             {}
func (w *window) SetMinimumSize(platform.Size)               {}
func (w *window) SetMaximumSize(platform.Size)               {}
func (w *window) SetResizable(bool)                          {}
func (w *window) IsResizable() bool                          { return false }
func (w *window) SetMovable(bool)                            {}
func (w *window) IsMovable() bool                            { return false }
func (w *window) SetMinimizable(bool)                        {}
func (w *window) IsMinimizable() bool                        { return false }
func (w *window) SetMaximizable(bool)                        {}
func (w *window) IsMaximizable() bool                        { return false }
func (w *window) SetClosable(bool)                           {}
func (w *window) IsClosable() bool                           { return true }
func (w *window) SetAlwaysOnTop(bool)                        {}
func (w *window) IsAlwaysOnTop() bool                        { return false }
func (w *window) Minimize()                                  {}
func (w *window) IsMinimized() bool                          { return false }
func (w *window) Maximize()                                  {}
func (w *window) Unmaximize()                                {}
func (w *window) IsMaximized() bool                          { return true }
func (w *window) Restore()                                   {}
func (w *window) SetFullScreen(bool)                         {}
func (w *window) IsFullScreen() bool                         { return true }
func (w *window) Center()                                    {}
func (w *window) SetOpacity(float64)                         {}
func (w *window) Opacity() float64                           { return 1 }
func (w *window) SetHasShadow(bool)                          {}
func (w *window) HasShadow() bool                            { return false }
func (w *window) SetIgnoreMouseEvents(bool)                  {}
func (w *window) IgnoresMouseEvents() bool                   { return false }
func (w *window) SetVibrancyTint(*platform.Color)            {}
func (w *window) SetContentProtection(bool)                  {}
func (w *window) SetVibrancy(string)                         {}
func (w *window) SetActiveAppearance(bool)                   {}
func (w *window) SetWindowButtonsTheme(string)               {}
func (w *window) SetProgressBar(string, float64)             {}
func (w *window) FlashFrame(bool)                            {}
func (w *window) SetSkipTaskbar(bool)                        {}
func (w *window) SetVisibleOnAllWorkspaces(bool)             {}
func (w *window) SetIcon([]byte) error                       { return nil }
func (w *window) SetMenu(*platform.Menu)                     {}
func (w *window) SetAutoHideMenu(bool)                       {}
func (w *window) TitleBar() platform.TitleBar                { return platform.TitleBar{} }
func (w *window) StartDrag()                                 {}
func (w *window) TitleBarDoubleClicked()                     {}
func (w *window) PlaceFlyout(platform.Flyout, platform.Size) {}

// Bounds is the screen's, as the window fills it.
func (w *window) Bounds() platform.Rect {
	return platform.Rect{Width: int(w.width + 0.5), Height: int(w.height + 0.5)}
}

func (w *window) ContentBounds() platform.Rect { return w.Bounds() }

func (w *window) Show() {
	if w.closed {
		return
	}
	old := w.b.front()
	w.b.unshow(w)
	w.b.shown = append(w.b.shown, w)
	w.visible = true
	w.b.call(&wire.ShowWindow{ID: w.id}, nil)
	w.b.refocus(old)
}

func (w *window) ShowInactive() { w.Show() }

// unshow takes w out of the windows on screen.
func (b *Backend) unshow(w *window) {
	for i, s := range b.shown {
		if s == w {
			b.shown = append(b.shown[:i], b.shown[i+1:]...)
			return
		}
	}
}

func (w *window) Hide() {
	if !w.visible || w.closed {
		return
	}
	old := w.b.front()
	w.b.unshow(w)
	w.visible = false
	w.b.call(&wire.HideWindow{ID: w.id}, nil)
	w.b.refocus(old)
}

func (w *window) IsVisible() bool { return w.visible }

func (w *window) Focus() {
	if w.parent == nil && w.visible && w.b.front() != w {
		w.Show()
	}
	w.b.call(&wire.Focus{ID: w.id}, nil)
}

func (w *window) Blur()           {}
func (w *window) IsFocused() bool { return w.focused }

// CornerRadius is 0: the screen's corners are the system's, which it does
// not tell.
func (w *window) CornerRadius() float64 { return 0 }

func (w *window) SetBackgroundColor(c platform.Color) {
	w.bg = c
	w.b.call(&wire.SetBackground{ID: w.id, Color: rgba(c)}, nil)
}

func (w *window) Close() {
	if w.closed {
		return
	}
	for _, c := range append([]*window(nil), w.children...) {
		c.Close()
	}
	if p := w.parent; p != nil {
		// A web view: its handler hears no more of it.
		p.children = slices.DeleteFunc(p.children, func(c *window) bool { return c == w })
		w.closed = true
		w.b.call(&wire.CloseWindow{ID: w.id}, nil)
		delete(w.b.windows, w.id)
		return
	}
	old := w.b.front()
	w.b.unshow(w)
	w.closed, w.visible = true, false
	w.b.call(&wire.CloseWindow{ID: w.id}, nil)
	delete(w.b.windows, w.id)
	if w.s != nil {
		w.s.closed()
	}
	w.b.refocus(old)
	w.h.Closed()
}

// TabInto gives a web view the keyboard: phones' keyboards have no Tab to
// come in with.
func (w *window) TabInto(bool) { w.b.call(&wire.Focus{ID: w.id}, nil) }

// resized takes the size the host reported.
func (w *window) resized(e *wire.ResizedEvent) {
	changed := e.Width != w.width || e.Height != w.height || e.Scale != w.scale
	w.width, w.height, w.scale = e.Width, e.Height, e.Scale
	insets := platform.Insets{Top: e.SafeTop, Left: e.SafeLeft, Bottom: e.SafeBottom, Right: e.SafeRight}
	inset := insets != w.insets || e.Keyboard != w.keyboard
	w.insets, w.keyboard = insets, e.Keyboard
	if w.s != nil {
		w.s.resized(changed, inset)
	}
	if changed {
		w.h.Resized()
	}
}
