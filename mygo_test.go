//go:build !js

package mygo

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/fake"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/transfer"
)

var fb *fake.Backend

// TestMain runs the application on the main goroutine with the fake
// backend, like a real program, and the tests on another goroutine.
func TestMain(m *testing.M) {
	if os.Getenv("MYGO_TEST_SECOND_INSTANCE") == "1" {
		// Helper process for TestSingleInstance. Only an instance that
		// hands over passes the foreground on (Windows).
		helper := fake.New()
		backendOnce.Do(func() { theBackend = helper })
		packageIdentifier = os.Getenv("MYGO_TEST_IDENTIFIER")
		App.SetName(cmp.Or(os.Getenv("MYGO_TEST_INSTANCE_NAME"), "MyGoTest"))
		singleInstanceTestHelper(helper)
		os.Exit(0)
	}
	if os.Getenv("MYGO_TEST_INSTANCE_CHILD") == "1" {
		// A process that an instance started, which outlives it.
		fmt.Println("child started")
		_, _ = io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}
	if dir := os.Getenv("MYGO_TEST_CRASH_LOG"); dir != "" {
		// Helper process for TestCrashLog: a packaged app that panics.
		App.SetPath(PathLogs, dir)
		logCrashes()
		panic("mygo test crash")
	}
	fb = fake.New()
	backendOnce.Do(func() { theBackend = fb })
	if os.Getenv("MYGO_TEST_STUCK_QUIT") == "1" {
		stuckQuit()
	}
	if dir := os.Getenv("MYGO_TEST_CLEAR_WINDOW_STATES"); dir != "" {
		clearWindowStatesOnQuit(dir)
	}
	if mode := os.Getenv("MYGO_TEST_END_SESSION"); mode != "" {
		endSession(mode)
	}
	// A quit that a test lets through must not end the tests.
	stopGrace = 0
	if os.Getenv("MYGO_TEST_BEFORE_RUN") == "1" {
		beforeRun()
	}
	App.SetName("MyGoTest")
	// Keep running when tests close their windows.
	App.OnWindowAllClosed(func() {})
	App.WhenReady(func() {
		go func() { os.Exit(m.Run()) }()
	})
	if err := App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// testWindow creates a window and returns it with its fake native window.
func testWindow(t *testing.T, opts WindowOptions) (*Window, *fake.Window) {
	t.Helper()
	w := NewWindow(opts)
	t.Cleanup(w.Destroy)
	wins := fb.Windows()
	return w, wins[len(wins)-1]
}

// A window made with IgnoreMouseEvents lets the mouse through from the
// start, as one given SetIgnoreMouseEvents(true) does later.
func TestWindowIgnoreMouseEvents(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	if w.IgnoresMouseEvents() {
		t.Error("a window takes the mouse by default, and says it ignores it")
	}
	w.SetIgnoreMouseEvents(true)
	if !w.IgnoresMouseEvents() {
		t.Error("IgnoresMouseEvents is false after SetIgnoreMouseEvents(true)")
	}
	w.SetIgnoreMouseEvents(false)
	if w.IgnoresMouseEvents() {
		t.Error("IgnoresMouseEvents is true after SetIgnoreMouseEvents(false)")
	}

	through, fw := testWindow(t, WindowOptions{IgnoreMouseEvents: true})
	if !fw.IgnoresMouseEvents() {
		t.Error("the backend's window made with IgnoreMouseEvents takes the mouse")
	}
	if !through.IgnoresMouseEvents() {
		t.Error("IgnoresMouseEvents is false for a window made with IgnoreMouseEvents")
	}
	through.Destroy()
	if through.IgnoresMouseEvents() {
		t.Error("a destroyed window says it ignores the mouse")
	}
}

func TestWindowDefaults(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	o := fw.Opts
	if o.Width != 800 || o.Height != 600 || !o.Center || o.Title != "MyGoTest" {
		t.Errorf("unexpected defaults: %+v", o)
	}
	if !o.Resizable || !o.Closable || !o.Minimizable || !o.Maximizable || !o.HasShadow || o.Opacity != 1 {
		t.Errorf("unexpected capability defaults: %+v", o)
	}
	if !o.DevTools {
		t.Error("devtools should be enabled in development")
	}
	if !w.IsVisible() {
		t.Error("window should be shown by default")
	}
	if WindowByID(w.ID()) != w {
		t.Error("WindowByID")
	}
}

// TestMinimizedWindowIsVisible holds IsVisible to what Show and Hide set,
// as every backend reports it (#1542): a minimized window is still shown,
// Show does not show it again, and Hide hides it.
func TestMinimizedWindowIsVisible(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	var shows, hides atomic.Int32
	w.OnShow(func() { shows.Add(1) })
	w.OnHide(func() { hides.Add(1) })
	w.Minimize()
	if !w.IsMinimized() || !w.IsVisible() {
		t.Fatalf("minimized: IsMinimized %v, IsVisible %v", w.IsMinimized(), w.IsVisible())
	}
	w.ShowInactive()
	if shows.Load() != 0 {
		t.Errorf("ShowInactive on a minimized window fired OnShow %d times", shows.Load())
	}
	w.Hide()
	if w.IsVisible() || hides.Load() != 1 {
		t.Errorf("Hide on a minimized window: IsVisible %v, OnHide %d times", w.IsVisible(), hides.Load())
	}
}

// TestGlassWindowOptions passes a floating panel's look to the backend: the
// glass and the corners of a frameless window, a negative radius none; and
// its active look, which SetActiveAppearance sets.
func TestGlassWindowOptions(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Frameless: true, Transparent: true, Vibrancy: VibrancyGlassClear, CornerRadius: 16})
	if o := fw.Opts; o.Vibrancy != "glass-clear" || o.CornerRadius != 16 {
		t.Errorf("vibrancy %q, radius %d", o.Vibrancy, o.CornerRadius)
	}
	if fw.ActiveAppearance() {
		t.Error("drawn as active before SetActiveAppearance")
	}
	w.SetActiveAppearance(true)
	if !fw.ActiveAppearance() {
		t.Error("SetActiveAppearance(true) did not reach the backend")
	}
	_, fw = testWindow(t, WindowOptions{Frameless: true, CornerRadius: -4})
	if o := fw.Opts; o.CornerRadius != 0 {
		t.Errorf("radius %d; want none", o.CornerRadius)
	}
}

// TestInputRegion passes a desktop pet's region to the backend, a copy
// without the rectangles that hold nothing, keeping nil (all of the
// window) apart from an empty region (none of it).
func TestInputRegion(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Frameless: true, Transparent: true, AlwaysOnTop: true})
	region := func() []platform.Rect {
		return onMainValue(func() []platform.Rect { return fw.InputRegion })
	}
	if region() != nil {
		t.Fatalf("a new window has region %v", region())
	}
	rects := []Rectangle{{X: 10, Y: 80, Width: 120, Height: 140}, {X: 0, Y: 0, Width: 0, Height: 40}, {X: 5, Y: 5, Width: 160, Height: -1}, {X: 0, Y: 0, Width: 200, Height: 60}}
	w.SetInputRegion(rects)
	rects[0].X = 99 // the caller's slice is its own
	if got, want := region(), []platform.Rect{{X: 10, Y: 80, Width: 120, Height: 140}, {X: 0, Y: 0, Width: 200, Height: 60}}; !reflect.DeepEqual(got, want) {
		t.Errorf("region %v, want %v", got, want)
	}
	w.SetInputRegion([]Rectangle{{Width: 0, Height: 10}})
	if got := region(); got == nil || len(got) != 0 {
		t.Errorf("region %#v, want an empty one", got)
	}
	w.SetInputRegion(nil)
	if got := region(); got != nil {
		t.Errorf("region %v, want nil", got)
	}
}

func TestWindowOptions(t *testing.T) {
	parent, _ := testWindow(t, WindowOptions{})
	_, fw := testWindow(t, WindowOptions{
		Title: "Custom", Width: 300, Height: 200, X: 10, Y: 20,
		Hidden: true, DisableResize: true, BackgroundColor: "#11223380",
		DevTools: DevToolsDisabled, Parent: parent,
	})
	o := fw.Opts
	if o.Title != "Custom" || o.Width != 300 || o.Center || o.Resizable || o.DevTools {
		t.Errorf("options not applied: %+v", o)
	}
	if c := o.BackgroundColor; c == nil || *c != (platform.Color{R: 0x11, G: 0x22, B: 0x33, A: 0x80}) {
		t.Errorf("background color: %v", c)
	}
	if o.Parent == nil {
		t.Error("parent not passed")
	}
	if fw.IsVisible() {
		t.Error("hidden window was shown")
	}
}

func TestConsumeActivationClickOption(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		_, fw := testWindow(t, WindowOptions{ConsumeActivationClick: enabled})
		if fw.Opts.ConsumeActivationClick != enabled {
			t.Errorf("ConsumeActivationClick = %v, want %v", fw.Opts.ConsumeActivationClick, enabled)
		}
	}
}

func TestInvalidBackgroundColor(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for invalid color")
		}
	}()
	NewWindow(WindowOptions{BackgroundColor: "nope"})
}

func TestBackgroundFollowsTheme(t *testing.T) {
	light, dark := platform.Color{R: 0xf6, G: 0xf7, B: 0xf9, A: 255}, platform.Color{R: 0x0f, G: 0x11, B: 0x15, A: 255}
	w, fw := testWindow(t, WindowOptions{BackgroundColor: "light-dark(#f6f7f9, rgb(15, 17, 21))"})
	if c := fw.Opts.BackgroundColor; c == nil || *c != light {
		t.Errorf("initial background: %v", c)
	}
	Theme.SetSource(ThemeDark)
	defer Theme.SetSource(ThemeSystem)
	if fw.Background != dark {
		t.Errorf("dark background: %v", fw.Background)
	}
	_, dfw := testWindow(t, WindowOptions{BackgroundColor: "light-dark(#f6f7f9, #0f1115)"})
	if c := dfw.Opts.BackgroundColor; c == nil || *c != dark {
		t.Errorf("background of a window created in dark mode: %v", c)
	}
	_, tfw := testWindow(t, WindowOptions{Transparent: true, BackgroundColor: "light-dark(#f6f7f9, #0f1115)"})
	if err := w.SetBackgroundColor("light-dark(#fff, #000)"); err != nil || fw.Background != (platform.Color{A: 255}) {
		t.Errorf("SetBackgroundColor: %v, %v", fw.Background, err)
	}
	Theme.SetSource(ThemeLight)
	if fw.Background != (platform.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("light background: %v", fw.Background)
	}
	if tfw.Background != (platform.Color{}) {
		t.Errorf("a theme change gave a transparent window a background: %v", tfw.Background)
	}
	if err := w.SetBackgroundColor("light-dark(#fff)"); err == nil {
		t.Error("SetBackgroundColor accepted an invalid color")
	}
}

func TestCloseCanBePrevented(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var allow atomic.Bool
	var closed atomic.Int32
	w.OnClose(func(e *CloseEvent) {
		if e.Window != w {
			t.Error("wrong window in CloseEvent")
		}
		if !allow.Load() {
			e.PreventDefault()
		}
	})
	w.OnClosed(func() { closed.Add(1) })

	w.Close()
	if w.IsDestroyed() || fw.IsClosed() {
		t.Fatal("close was not prevented")
	}
	if onMainValue(fw.UserClose) {
		t.Fatal("user close was not prevented")
	}
	allow.Store(true)
	w.Close()
	if !w.IsDestroyed() || !fw.IsClosed() || closed.Load() != 1 {
		t.Fatalf("window not closed: destroyed=%v closed=%v events=%d", w.IsDestroyed(), fw.IsClosed(), closed.Load())
	}
	for _, x := range Windows() {
		if x == w {
			t.Error("closed window still listed")
		}
	}
	// Methods on destroyed windows are no-ops.
	w.SetTitle("ignored")
	if w.Title() != "" || w.IsVisible() {
		t.Error("destroyed window should report zero values")
	}
}

func TestDestroyClosesChildren(t *testing.T) {
	parent, _ := testWindow(t, WindowOptions{})
	child, _ := testWindow(t, WindowOptions{Parent: parent})
	parent.Destroy()
	if !child.IsDestroyed() {
		t.Error("child window survived its parent")
	}
}

func TestWindowStateAndEvents(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	var events []string
	var mu sync.Mutex
	record := func(name string) func() {
		return func() { mu.Lock(); events = append(events, name); mu.Unlock() }
	}
	w.OnMaximize(record("maximize"))
	w.OnUnmaximize(record("unmaximize"))
	w.OnMinimize(record("minimize"))
	w.OnRestore(record("restore"))
	w.OnResize(record("resize"))
	w.OnFocus(record("focus"))

	w.ToggleMaximize()
	if !w.IsMaximized() {
		t.Error("ToggleMaximize did not maximize")
	}
	w.ToggleMaximize()
	w.Minimize()
	w.Restore()
	w.SetSize(640, 480)
	w.Focus()
	if FocusedWindow() != w {
		t.Error("FocusedWindow")
	}
	if width, height := w.Size(); width != 640 || height != 480 {
		t.Errorf("Size = %d,%d", width, height)
	}
	mu.Lock()
	defer mu.Unlock()
	want := "maximize,unmaximize,minimize,restore,resize,focus"
	if got := strings.Join(events, ","); got != want {
		t.Errorf("events = %s, want %s", got, want)
	}
}

func TestWindowAllClosedOnce(t *testing.T) {
	if n := len(Windows()); n != 0 {
		t.Skipf("%d windows are open", n)
	}
	var count atomic.Int32
	off := App.OnWindowAllClosed(func() { count.Add(1) })
	defer off()
	parent := NewWindow(WindowOptions{})
	child := NewWindow(WindowOptions{Parent: parent})
	NewWindow(WindowOptions{Parent: child})
	parent.Close()
	onMain(func() {})
	if n := len(Windows()); n != 0 {
		t.Fatalf("%d windows left", n)
	}
	if n := count.Load(); n != 1 {
		t.Errorf("OnWindowAllClosed fired %d times", n)
	}
}

// Menus.

func TestMenuRolesAndState(t *testing.T) {
	var clicks []string
	var mu sync.Mutex
	click := func(item *MenuItem, win *Window) {
		mu.Lock()
		clicks = append(clicks, item.ID)
		mu.Unlock()
	}
	menu := NewMenu([]*MenuItem{
		{Label: "File", Submenu: []*MenuItem{
			{ID: "new", Label: "New", Accelerator: "CmdOrCtrl+N", Click: click},
			Separator(),
			{Role: RoleQuit},
		}},
		{Label: "View", Submenu: []*MenuItem{
			{ID: "grid", Label: "Grid", Type: MenuItemCheckbox, Click: click},
			{ID: "small", Label: "Small", Type: MenuItemRadio, Checked: true},
			{ID: "large", Label: "Large", Type: MenuItemRadio},
		}},
		{Role: RoleEditMenu},
	})
	snap := menu.snapshot()
	if snap.Items[0].Type != platform.MenuItemSubmenu || snap.Items[0].Submenu.Items[1].Type != platform.MenuItemSeparator {
		t.Errorf("types not inferred: %+v", snap.Items[0])
	}
	quit := snap.Items[0].Submenu.Items[2]
	if !strings.HasPrefix(quit.Label, "Quit") && quit.Label != "Exit" {
		t.Errorf("quit label = %q", quit.Label)
	}
	edit := snap.Items[2]
	if edit.Label != "Edit" || len(edit.Submenu.Items) < 6 || edit.Submenu.Items[0].Accelerator != "CmdOrCtrl+Z" {
		t.Errorf("edit menu not expanded: %+v", edit)
	}

	grid := menu.ItemByID("grid")
	small, large := menu.ItemByID("small"), menu.ItemByID("large")
	onMain(func() { fb.ClickMenuItem(grid.uid) })
	if !grid.IsChecked() {
		t.Error("checkbox not toggled")
	}
	onMain(func() { fb.ClickMenuItem(large.uid) })
	if !large.IsChecked() || small.IsChecked() {
		t.Error("radio group not exclusive")
	}
	onMain(func() { fb.ClickMenuItem(menu.ItemByID("new").uid) })
	mu.Lock()
	if strings.Join(clicks, ",") != "grid,new" {
		t.Errorf("clicks = %v", clicks)
	}
	mu.Unlock()

	menu.ItemByID("new").SetEnabled(false)
	onMain(func() {}) // the update reaches the backend on the main thread
	updates := fb.MenuUpdates()
	last := updates[len(updates)-1]
	if last.ID != menu.ItemByID("new").uid || last.Enabled {
		t.Errorf("SetEnabled not propagated: %+v", last)
	}
}

func TestMenuBadge(t *testing.T) {
	menu := NewMenu([]*MenuItem{{
		ID: "node", Label: "Node", Badge: "120 ms", BadgeAlign: BadgeAlignLeft,
		BadgeShape: BadgeShapePill, BadgeColor: "#34c759", BadgeTextColor: "#fff",
	}})
	snap := menu.snapshot()
	if got := snap.Items[0].Badge; got != "120 ms" {
		t.Fatalf("badge = %q", got)
	}
	if got := snap.Items[0].BadgeAlign; got != platform.BadgeAlignLeft {
		t.Fatalf("badge align = %q", got)
	}
	if pill := snap.Items[0]; pill.BadgeShape != platform.BadgeShapePill || pill.BadgeColor == nil ||
		*pill.BadgeColor != (platform.Color{R: 0x34, G: 0xc7, B: 0x59, A: 255}) ||
		pill.BadgeTextColor == nil || *pill.BadgeTextColor != (platform.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("badge pill = %+v", pill)
	}
	item := menu.ItemByID("node")
	item.SetBadge("1.2s")
	item.SetBadgeAlign(BadgeAlignCenter)
	item.SetBadgeShape("")
	item.SetBadgeColor("")
	item.SetBadgeTextColor("#000")
	onMain(func() {}) // the updates reach the backend on the main thread
	updates := fb.MenuUpdates()
	last := updates[len(updates)-1]
	if last.ID != item.uid || last.Badge != "1.2s" || last.BadgeAlign != platform.BadgeAlignCenter ||
		last.BadgeShape != "" || last.BadgeColor != nil ||
		last.BadgeTextColor == nil || *last.BadgeTextColor != (platform.Color{R: 0, G: 0, B: 0, A: 255}) {
		t.Errorf("badge updates not propagated: %+v", last)
	}
}

func TestMenuKeepOpen(t *testing.T) {
	menu := NewMenu([]*MenuItem{{ID: "node", Label: "Node", KeepOpen: true}})
	if !menu.snapshot().Items[0].KeepOpen {
		t.Error("KeepOpen lost in the snapshot")
	}
}

func TestDefaultMenu(t *testing.T) {
	m := fb.AppMenu()
	if runtime.GOOS != "darwin" {
		if m != nil {
			t.Error("no default menu expected outside macOS")
		}
		return
	}
	if m == nil || len(m.Items) != 5 || m.Items[0].Label != "MyGoTest" {
		t.Fatalf("default menu: %+v", m)
	}
}

func TestWindowMenuActivation(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Hidden: true})
	other, otherNative := testWindow(t, WindowOptions{Hidden: true})
	other.Focus()
	var clicked *Window
	var checked, minimized bool
	clicks := 0
	menu := NewMenu([]*MenuItem{{Label: "Window", Submenu: []*MenuItem{
		{ID: "small", Label: "Small", Type: MenuItemRadio, Checked: true},
		{ID: "min", Label: "Minimize", Type: MenuItemRadio, Role: RoleMinimize, Click: func(it *MenuItem, win *Window) {
			clicked, checked = win, it.IsChecked()
			if win != nil {
				minimized = win.IsMinimized()
			}
			clicks++
		}},
	}}})
	item := menu.ItemByID("min")
	onMain(func() { fw.ClickMenuItem(item.uid) })
	if clicked != w || !checked || !minimized || menu.ItemByID("small").IsChecked() {
		t.Errorf("click = (%v, checked %v, minimized %v), want the menu's window after its state and role changed", clicked, checked, minimized)
	}
	if other.IsMinimized() {
		t.Error("the role changed the focused window instead of the menu's window")
	}
	if FocusedWindow() != other {
		t.Error("choosing a menu item changed window focus")
	}

	// A submenu taking focus clears FocusedWindow; its owner still applies.
	onMain(otherNative.Blur)
	if FocusedWindow() != nil {
		t.Fatal("a window still has focus")
	}
	onMain(func() { fw.ClickMenuItem(item.uid) })
	if clicked != w || !minimized {
		t.Errorf("click without focus = (%v, minimized %v), want the menu's window", clicked, minimized)
	}

	// Application and tray menus without a window use the focused window.
	other.Focus()
	onMain(func() { fb.ClickMenuItem(item.uid) })
	if clicked != other || !other.IsMinimized() {
		t.Errorf("application click = (%v, minimized %v), want the focused window", clicked, other.IsMinimized())
	}
	onMain(otherNative.Blur)
	onMain(func() { fb.ClickMenuItem(item.uid) })
	if clicked != nil {
		t.Error("a menu without a window supplied one while none had focus")
	}

	w.Destroy()
	before := clicks
	onMain(func() { fw.ClickMenuItem(item.uid) })
	if clicks != before {
		t.Error("a destroyed window still delivered a menu click")
	}
}

// Application.

func TestQuitCanBeCanceled(t *testing.T) {
	w, _ := testWindow(t, WindowOptions{})
	off := App.OnBeforeQuit(func(e *QuitEvent) { e.PreventDefault() })
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by OnBeforeQuit")
	}
	off()
	offClose := w.OnClose(func(e *CloseEvent) { e.PreventDefault() })
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by a window")
	}
	offClose()
	offWill := App.OnWillQuit(func(e *QuitEvent) { e.PreventDefault() })
	defer offWill()
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by OnWillQuit")
	}
	if !w.IsDestroyed() {
		t.Error("windows are closed before OnWillQuit")
	}
}

func TestQuitCanceledEvent(t *testing.T) {
	var canceled int
	off := App.OnQuitCanceled(func() { canceled++ })
	defer off()

	offBefore := App.OnBeforeQuit(func(e *QuitEvent) { e.PreventDefault() })
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by OnBeforeQuit")
	}
	offBefore()

	w, _ := testWindow(t, WindowOptions{})
	offClose := w.OnClose(func(e *CloseEvent) { e.PreventDefault() })
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by a window")
	}
	offClose()

	offWill := App.OnWillQuit(func(e *QuitEvent) { e.PreventDefault() })
	if onMainValue(fb.QuitRequested) {
		t.Fatal("quit was not canceled by OnWillQuit")
	}
	offWill()

	if canceled != 3 {
		t.Errorf("OnQuitCanceled ran %d times, want 3", canceled)
	}
}

func TestPaths(t *testing.T) {
	dir, err := App.Path(PathUserData)
	if err != nil || !strings.HasSuffix(dir, "MyGoTest") {
		t.Fatalf("userData = %q, %v", dir, err)
	}
	defer os.Remove(dir)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Error("userData was not created")
	}
	App.SetPath(PathDownloads, "/tmp/dl")
	if p, _ := App.Path(PathDownloads); p != "/tmp/dl" {
		t.Errorf("override = %q", p)
	}
	if _, err := App.Path("nope"); err == nil {
		t.Error("unknown path should fail")
	}
	wd, _ := os.Getwd()
	exe, _ := os.Executable()
	if p, err := App.Path(PathResources); err != nil || p != resourcesDir(exe, wd) {
		t.Errorf("resources = %q, %v", p, err)
	}
	for _, c := range []struct{ exe, want string }{
		{"/Applications/My App.app/Contents/MacOS/My App", "/Applications/My App.app/Contents/Resources"},
		{"/opt/my-app/my-app", "/opt/my-app"},
		{`/tmp/go-build123/b001/exe/app`, "/work/resources"},
		{`/tmp/go-build123/b001/app.test`, "/work/resources"},
	} {
		exe, want := filepath.FromSlash(c.exe), filepath.FromSlash(c.want)
		if got := resourcesDir(exe, filepath.FromSlash("/work")); got != want {
			t.Errorf("resourcesDir(%q) = %q, want %q", exe, got, want)
		}
	}
}

func TestModules(t *testing.T) {
	Clipboard.WriteText("copied")
	if Clipboard.ReadText() != "copied" {
		t.Error("clipboard")
	}
	if d := Screen.PrimaryDisplay(); d.Label != "Fake" || d.WorkArea.Y != 25 {
		t.Errorf("primary display = %+v", d)
	}
	if d := Screen.DisplayNearestPoint(Point{5000, 5000}); d.Label != "Fake" {
		t.Error("DisplayNearestPoint")
	}
	var themed atomic.Int32
	off := Theme.OnUpdated(func() { themed.Add(1) })
	defer off()
	Theme.SetSource(ThemeDark)
	if !Theme.IsDark() || Theme.Source() != ThemeDark || themed.Load() != 1 {
		t.Error("theme")
	}
	Theme.SetSource(ThemeSystem)

	fb.OpenResult = []string{"/a.txt"}
	if paths, err := Dialog.Open(OpenDialogOptions{}); err != nil || len(paths) != 1 {
		t.Errorf("Dialog.Open = %v, %v", paths, err)
	}
	fb.MessageResult = platform.MessageBoxResult{Response: 1, CheckboxChecked: true}
	if res, err := Dialog.Message(MessageOptions{Buttons: []string{"OK", "Cancel"}}); err != nil || res.Button != 1 || !res.CheckboxChecked {
		t.Errorf("Dialog.Message = %+v, %v", res, err)
	}
}

func TestThemePreferences(t *testing.T) {
	// A desktop that does not say: no accent, and the usual text.
	onMain(func() { fb.SetPreferences(platform.Preferences{}) })
	defer onMain(func() { fb.SetPreferences(platform.Preferences{}) })
	if p := Theme.Preferences(); p != (ThemePreferences{TextScale: 1}) {
		t.Errorf("Preferences without a desktop = %+v, want TextScale 1", p)
	}

	// A desktop with an accent and the settings of accessibility.
	onMain(func() {
		fb.SetPreferences(platform.Preferences{
			Accent:             platform.Color{R: 255, G: 128, B: 0, A: 255},
			ReduceMotion:       true,
			ReduceTransparency: true,
			HighContrast:       true,
			TextScale:          1.25,
		})
	})
	want := ThemePreferences{
		Accent:             color.NRGBA{R: 255, G: 128, B: 0, A: 255},
		ReduceMotion:       true,
		ReduceTransparency: true,
		HighContrast:       true,
		TextScale:          1.25,
	}
	if p := Theme.Preferences(); p != want {
		t.Errorf("Preferences = %+v, want %+v", p, want)
	}

	// Safe from any goroutine.
	done := make(chan ThemePreferences, 1)
	go func() { done <- Theme.Preferences() }()
	select {
	case p := <-done:
		if p != want {
			t.Errorf("Preferences from another goroutine = %+v, want %+v", p, want)
		}
	case <-time.After(wait.Guard):
		t.Fatal("Preferences from another goroutine hung")
	}
}

// TestThemePreferencesJSON checks the JSON of the preferences, which is
// mygo-runtime's ThemePreferences: the accent a CSS color or null.
// Theme.SetGTKCSS gives the backend the sheet, from any goroutine, each
// in place of the one before, and returns why one did not load (#1121).
func TestThemeGTKCSS(t *testing.T) {
	defer onMain(func() { fb.StyleSheet, fb.StyleSheetError = "", nil })
	sheet := func() (s string) {
		onMain(func() { s = fb.StyleSheet })
		return s
	}
	for _, css := range []string{"headerbar { min-height: 46px; }", "button.titlebutton { min-width: 24px; }"} {
		if err := Theme.SetGTKCSS(css); err != nil {
			t.Fatal(err)
		}
		if got := sheet(); got != css {
			t.Errorf("sheet = %q, want %q", got, css)
		}
	}

	parse := errors.New("<data>:1:29: not a number")
	onMain(func() { fb.StyleSheetError = parse })
	err := Theme.SetGTKCSS("button.titlebutton { min-width: nonsense; }")
	if !errors.Is(err, parse) || err.Error() != "mygo: Theme.SetGTKCSS: <data>:1:29: not a number" {
		t.Errorf("SetGTKCSS of a sheet that does not load = %v, want the backend's error", err)
	}
	if got := sheet(); got != "button.titlebutton { min-width: 24px; }" {
		t.Errorf("sheet after one that does not load = %q, want the one before", got)
	}
}

// Theme.WindowButtons reports the desktop's layout, which is empty under a
// tiling window manager.
func TestThemeWindowButtons(t *testing.T) {
	defer onMain(func() { fb.StartButtons, fb.EndButtons = nil, nil })
	onMain(func() { fb.StartButtons, fb.EndButtons = []string{"close"}, []string{"minimize", "maximize"} })
	b := Theme.WindowButtons()
	if !slices.Equal(b.Start, []string{"close"}) || !slices.Equal(b.End, []string{"minimize", "maximize"}) {
		t.Errorf("WindowButtons = %+v, want the backend's layout", b)
	}
	if b.Empty() || !b.Has("close") || !b.Has("maximize") {
		t.Errorf("Empty, Has of %+v", b)
	}
	onMain(func() { fb.StartButtons, fb.EndButtons = nil, nil })
	if b := Theme.WindowButtons(); !b.Empty() || b.Has("close") {
		t.Errorf("WindowButtons with none = %+v, want empty", b)
	}
	if b := defaultWindowButtons("windows"); !slices.Equal(b.End, []string{"minimize", "maximize", "close"}) || len(b.Start) != 0 {
		t.Errorf("defaultWindowButtons(windows) = %+v", b)
	}
	if b := defaultWindowButtons("darwin"); !slices.Equal(b.Start, []string{"close", "minimize", "maximize"}) {
		t.Errorf("defaultWindowButtons(darwin) = %+v", b)
	}
}

func TestThemePreferencesJSON(t *testing.T) {
	for _, tt := range []struct {
		p    ThemePreferences
		json string
	}{
		{ThemePreferences{TextScale: 1}, `{"accent":null,"reduceMotion":false,"reduceTransparency":false,"highContrast":false,"textScale":1}`},
		{ThemePreferences{Accent: color.NRGBA{R: 255, G: 128, A: 255}, ReduceMotion: true, ReduceTransparency: true, HighContrast: true, TextScale: 1.5},
			`{"accent":"#ff8000","reduceMotion":true,"reduceTransparency":true,"highContrast":true,"textScale":1.5}`},
		{ThemePreferences{Accent: color.NRGBA{R: 255, G: 128, A: 128}, TextScale: 1}, `{"accent":"#ff800080","reduceMotion":false,"reduceTransparency":false,"highContrast":false,"textScale":1}`},
	} {
		b, err := json.Marshal(tt.p)
		if err != nil || string(b) != tt.json {
			t.Errorf("Marshal(%+v) = %s, %v, want %s", tt.p, b, err, tt.json)
			continue
		}
		var back ThemePreferences
		if err := json.Unmarshal(b, &back); err != nil || back != tt.p {
			t.Errorf("Unmarshal(%s) = %+v, %v, want %+v", b, back, err, tt.p)
		}
	}
}

// TestThemePreferencesAreReadOnce checks that the desktop is asked for its
// preferences once per change, as reading them can call out to it (the
// settings portal on Linux).
func TestThemePreferencesAreReadOnce(t *testing.T) {
	onMain(func() { fb.SetPreferences(platform.Preferences{Accent: platform.Color{R: 1, G: 2, B: 3, A: 255}}) })
	defer onMain(func() { fb.SetPreferences(platform.Preferences{}) })
	var changed atomic.Int32
	off := Theme.OnUpdated(func() { changed.Add(1) })
	defer off()

	if p := Theme.Preferences(); p.Accent.R != 1 {
		t.Fatalf("Preferences = %+v", p)
	}
	before := fb.PreferenceReads()
	for i := 0; i < 3; i++ {
		if p := Theme.Preferences(); p.Accent.R != 1 {
			t.Fatalf("Preferences = %+v", p)
		}
	}
	if n := fb.PreferenceReads() - before; n != 0 {
		t.Errorf("Preferences read the desktop %d more times, want 0", n)
	}

	// A change invalidates what was read, and tells the listeners once.
	onMain(func() { fb.SetPreferences(platform.Preferences{Accent: platform.Color{G: 9, A: 255}, TextScale: 2}) })
	if p := Theme.Preferences(); p.Accent.G != 9 || p.TextScale != 2 {
		t.Errorf("Preferences after a change = %+v", p)
	}
	if n := changed.Load(); n != 1 {
		t.Errorf("OnUpdated fired %d times, want 1", n)
	}
	before = fb.PreferenceReads()
	_ = Theme.Preferences()
	if n := fb.PreferenceReads() - before; n != 0 {
		t.Errorf("Preferences read the desktop %d times after a change, want 0", n)
	}
}

// needsAppCalls are the calls that need the running app, which main may
// not make before Run.
var needsAppCalls = []struct {
	name string
	call func()
}{
	{"App.Focus", func() { App.Focus() }},
	{"App.Hide", func() { App.Hide() }},
	{"App.Show", func() { App.Show() }},
	{"App.IsHidden", func() { App.IsHidden() }},
	{"App.SetBadgeCount", func() { App.SetBadgeCount(1) }},
	{"App.ShowAboutPanel", func() { App.ShowAboutPanel(AboutPanelOptions{}) }},
	{"App.Dock.SetBadge", func() { App.Dock.SetBadge("1") }},
	{"App.Dock.Badge", func() { App.Dock.Badge() }},
	{"App.Dock.Bounce", func() { App.Dock.Bounce(false) }},
	{"App.Dock.CancelBounce", func() { App.Dock.CancelBounce(1) }},
	{"App.Dock.SetIcon", func() { App.Dock.SetIcon(nil) }},
	{"Shell.OpenExternal", func() { Shell.OpenExternal("https://example.com") }},
	{"Shell.OpenPath", func() { Shell.OpenPath("/") }},
	{"Shell.ShowItemInFolder", func() { Shell.ShowItemInFolder("/") }},
	{"Shell.TrashItem", func() { Shell.TrashItem("/nonexistent") }},
	{"Shell.Beep", func() { Shell.Beep() }},
	{"Shell.FileIcon", func() { Shell.FileIcon("/", 16) }},
	{"Shell.TypeIcon", func() { Shell.TypeIcon("pdf", 16) }},
	{"Shell.InstalledApps", func() { Shell.InstalledApps() }},
	{"Shell.LaunchApp", func() { Shell.LaunchApp(InstalledApp{Path: "/"}) }},
	{"Clipboard.ReadText", func() { Clipboard.ReadText() }},
	{"Clipboard.WriteText", func() { Clipboard.WriteText("x") }},
	{"Clipboard.ReadHTML", func() { Clipboard.ReadHTML() }},
	{"Clipboard.WriteHTML", func() { Clipboard.WriteHTML("x") }},
	{"Clipboard.ReadImage", func() { Clipboard.ReadImage() }},
	{"Clipboard.WriteImage", func() { Clipboard.WriteImage(nil) }},
	{"Clipboard.Clear", func() { Clipboard.Clear() }},
	{"Clipboard.AvailableFormats", func() { Clipboard.AvailableFormats() }},
	{"Clipboard.Write", func() { Clipboard.Write(transfer.Data{}) }},
	{"Clipboard.Read", func() { Clipboard.Read() }},
	{"Clipboard.Formats", func() { Clipboard.Formats() }},
	{"Clipboard.ReadFormat", func() { Clipboard.ReadFormat(transfer.Text) }},
	{"Clipboard.ReadFiles", func() { Clipboard.ReadFiles() }},
	{"Clipboard.WriteFiles", func() { Clipboard.WriteFiles() }},
	{"Clipboard.Flush", func() { Clipboard.Flush() }},
	{"Screen.Displays", func() { Screen.Displays() }},
	{"Screen.PrimaryDisplay", func() { Screen.PrimaryDisplay() }},
	{"Screen.CursorScreenPoint", func() { Screen.CursorScreenPoint() }},
	{"Screen.DisplayNearestPoint", func() { Screen.DisplayNearestPoint(Point{}) }},
	{"Screen.DisplayMatching", func() { Screen.DisplayMatching(Rectangle{}) }},
	{"Theme.IsDark", func() { Theme.IsDark() }},
	{"Theme.Preferences", func() { Theme.Preferences() }},
	{"GlobalShortcut.Register", func() { GlobalShortcut.Register("CmdOrCtrl+Shift+K", func() {}) }},
	{"NewTray", func() { NewTray(TrayOptions{}) }},
	{"NotificationsSupported", func() { NotificationsSupported() }},
	{"Notification.Show", func() { NewNotification(NotificationOptions{}).Show() }},
	{"Power.KeepAwake", func() { Power.KeepAwake("test", false) }},
	{"Power.IdleTime", func() { Power.IdleTime() }},
	{"Network.Status", func() { Network.Status() }},
	{"Dialog.Open", func() { Dialog.Open(OpenDialogOptions{}) }},
	{"Dialog.Save", func() { Dialog.Save(SaveDialogOptions{}) }},
	{"Dialog.Message", func() { Dialog.Message(MessageOptions{}) }},
	{"Dialog.Error", func() { Dialog.Error("title", "content") }},
	{"Menu.Popup", func() { NewMenu(nil).Popup(nil) }},
	{"Menu.PopupAt", func() { NewMenu(nil).PopupAt(nil, 0, 0) }},
}

// TestQuitWhileLoopRuns: once the quit sequence has run, a loop that goes
// on running does not keep the app: a later quit asks it to stop again,
// and the app finishes and exits after stopGrace.
func TestQuitWhileLoopRuns(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "-test.run=^$")
	cmd.Env = append(os.Environ(), "MYGO_TEST_STUCK_QUIT=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if got := string(out); !strings.Contains(got, "asked again: 2\n") || !strings.Contains(got, "quit\n") {
		t.Errorf("output:\n%s", got)
	}
}

// TestEndSession: when the OS ends the session, the quit listeners and the
// windows' OnClose run before OnQuit, synchronously and in a quit's
// order, and what they prevent is ignored; after a quit that ran them,
// they do not run again.
func TestEndSession(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for mode, want := range map[string]string{
		"fresh": "before true\nclose\nwill true\nquit\nended\n",
		// The quit ran the sequence; its loop had not stopped yet.
		"quit": "before false\nclose\nwill false\nquit\nended\n",
	} {
		cmd := exec.Command(exe, "-test.run=^$")
		cmd.Env = append(os.Environ(), "MYGO_TEST_END_SESSION="+mode)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", mode, err, out)
		}
		if string(out) != want {
			t.Errorf("%s: output:\n%s\nwant:\n%s", mode, out, want)
		}
	}
}

// endSession is a helper process for TestEndSession: the OS ends the
// session while the app runs ("fresh"), when every listener tries to
// cancel, or after a quit ran its sequence and the loop has yet to stop
// ("quit").
func endSession(mode string) {
	fb.IgnoresQuit = true
	stopGrace = 0
	App.OnWindowAllClosed(func() { fmt.Println("all closed") })
	App.OnBeforeQuit(func(e *QuitEvent) {
		fmt.Println("before", e.SessionEnding)
		if mode == "fresh" {
			e.PreventDefault()
		}
	})
	App.OnWillQuit(func(e *QuitEvent) {
		fmt.Println("will", e.SessionEnding)
		if mode == "fresh" {
			e.PreventDefault()
		}
	})
	App.OnQuit(func() { fmt.Println("quit") })
	App.WhenReady(func() {
		w := NewWindow(WindowOptions{})
		w.OnClose(func(e *CloseEvent) {
			fmt.Println("close")
			if mode == "fresh" {
				e.PreventDefault()
			}
		})
		if mode == "quit" {
			App.Quit() // its loop does not stop (IgnoresQuit)
		}
		postMain(func() {
			fb.EndSession()
			if !w.IsDestroyed() {
				fmt.Println("the window is open")
			}
			fmt.Println("ended")
			os.Exit(0)
		})
	})
	App.Run()
	fmt.Println("the loop stopped")
	os.Exit(1)
}

// stuckQuit is a helper process for TestQuitWhileLoopRuns: its event
// loop does not stop when asked.
func stuckQuit() {
	fb.IgnoresQuit = true
	stopGrace = 100 * time.Millisecond
	App.OnWindowAllClosed(func() {})
	App.OnQuit(func() { fmt.Println("quit") })
	App.WhenReady(func() {
		App.Quit()
		App.Quit()
		postMain(func() { fmt.Println("asked again:", fb.Quits) })
	})
	App.Run()
	fmt.Println("the loop stopped")
	os.Exit(1)
}

// beforeRun is a helper process for TestBeforeRun: it does what main may
// do before Run, when the backend is not initialized, and what it may
// not, then exits.
func beforeRun() {
	// Settings, which Run applies.
	Theme.SetSource(ThemeDark)
	_ = Theme.SetGTKCSS("headerbar {}")
	App.Dock.SetMenu(NewMenu([]*MenuItem{{Label: "New Window"}}))
	// Calls that work without the running app.
	App.Locale()
	Power.IsOnBattery()
	GlobalShortcut.UnregisterAll()
	NewNotification(NotificationOptions{}).Close()
	// Nothing can be shown before Run, so there is nothing to clear and
	// no backend to ask.
	ClearNotifications()
	for _, c := range needsAppCalls {
		func() {
			defer func() { fmt.Printf("%s: %v\n", c.name, recover()) }()
			c.call()
		}()
	}
	App.WhenReady(func() {
		fmt.Println("dark:", Theme.IsDark())
		fmt.Println("gtk css:", fb.StyleSheet == "headerbar {}")
		fmt.Println("dock menu:", fb.DockMenu != nil && fb.DockMenu.Items[0].Label == "New Window")
		App.Quit()
	})
	if err := App.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	os.Exit(0)
}

// TestBeforeRun: main may make settings before Run, which apply once Run
// has initialized the backend, while calls that need the running app fail
// with a message naming them, the same on every platform rather than
// crashing, hanging or doing nothing depending on it. Generate mode,
// which never initializes the backend, still writes the client.
func TestBeforeRun(t *testing.T) {
	run := func(env ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^$")
		cmd.Env = append(append(os.Environ(), "MYGO_TEST_BEFORE_RUN=1"), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	out := run()
	wants := []string{"dark: true\n", "gtk css: true\n", "dock menu: true\n"}
	for _, c := range needsAppCalls {
		wants = append(wants, c.name+": mygo: "+c.name+" called before ")
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in the output:\n%s", want, out)
		}
	}
}

func TestGlobalShortcut(t *testing.T) {
	pressed := make(chan struct{}, 1)
	if err := GlobalShortcut.Register("CmdOrCtrl+Shift+K", func() { pressed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := GlobalShortcut.Register("Shift+CmdOrCtrl+k", func() {}); err == nil {
		t.Error("equivalent accelerator registered twice")
	}
	if !GlobalShortcut.IsRegistered("cmdorctrl+shift+k") {
		t.Error("IsRegistered")
	}
	onMain(func() { fb.PressHotkey("CmdOrCtrl+Shift+K") })
	select {
	case <-pressed:
	case <-time.After(wait.Guard):
		t.Error("shortcut callback not called")
	}
	GlobalShortcut.Unregister("CmdOrCtrl+Shift+K")
	if GlobalShortcut.IsRegistered("CmdOrCtrl+Shift+K") {
		t.Error("Unregister")
	}
}

// TestGlobalShortcutKeys: a desktop that binds shortcuts for apps hears
// what a shortcut does, and the shortcut hears the keys it got, once for
// each change, until it is unregistered.
func TestGlobalShortcutKeys(t *testing.T) {
	defer GlobalShortcut.UnregisterAll()
	const acc = "CmdOrCtrl+Alt+."
	var got []string
	opts := GlobalShortcutOptions{Description: "Stop all", OnKeys: func(keys string) { got = append(got, keys) }}
	if err := GlobalShortcut.RegisterWith(acc, opts, func() {}); err != nil {
		t.Fatal(err)
	}
	if err := GlobalShortcut.Register("CmdOrCtrl+Alt+,", func() {}); err != nil {
		t.Fatal(err)
	}
	onMain(func() {
		if d := fb.HotkeyDescription(acc); d != "Stop all" {
			t.Errorf("the desktop heard the description %q", d)
		}
		if d := fb.HotkeyDescription("CmdOrCtrl+Alt+,"); d != "" {
			t.Errorf("a shortcut without a description got %q", d)
		}
		fb.BindHotkey("CmdOrCtrl+Alt+,", "Ctrl+Alt+,") // nobody listens
		fb.BindHotkey(acc, "")                         // the user declined it
		fb.BindHotkey(acc, "")                         // the desktop answers again as other shortcuts change
		fb.BindHotkey(acc, "Meta+S")                   // the user picked keys in the desktop's settings
		fb.BindHotkey(acc, "Meta+S")
		fb.BindHotkey(acc, "")
	})
	if want := []string{"", "Meta+S", ""}; !slices.Equal(got, want) {
		t.Errorf("the shortcut heard the keys %q, want %q", got, want)
	}
	// A shortcut registered again starts over: it hears keys it heard before.
	got = nil
	GlobalShortcut.Unregister(acc)
	if err := GlobalShortcut.RegisterWith(acc, opts, func() {}); err != nil {
		t.Fatal(err)
	}
	onMain(func() { fb.BindHotkey(acc, "") })
	if want := []string{""}; !slices.Equal(got, want) {
		t.Errorf("registered again, the shortcut heard the keys %q, want %q", got, want)
	}
}

// TestGlobalShortcutRegisterRace: calls that register the same
// accelerator at once bind it once, and Unregister leaves nothing of the
// shortcut behind.
func TestGlobalShortcutRegisterRace(t *testing.T) {
	defer GlobalShortcut.UnregisterAll()
	const acc = "CmdOrCtrl+Alt+Shift+J"
	var bound, calls atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if err := GlobalShortcut.Register(acc, func() { calls.Add(1) }); err == nil {
				bound.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if n := bound.Load(); n != 1 {
		t.Errorf("Register(%q) succeeded %d times, want 1", acc, n)
	}
	GlobalShortcut.Unregister(acc)
	if GlobalShortcut.IsRegistered(acc) {
		t.Error("IsRegistered after Unregister")
	}
	onMain(func() { fb.PressHotkey(acc) })
	if n := calls.Load(); n != 0 {
		t.Errorf("a shortcut left bound by a lost race fired %d time(s) after Unregister", n)
	}
}

// TestNotificationShowFails: Show returns what the backend answers, and
// forgets a notification it could not show, which nothing will click.
func TestNotificationShowFails(t *testing.T) {
	onMain(func() { fb.NotificationError = ErrNotificationsDenied })
	t.Cleanup(func() { onMain(func() { fb.NotificationError = nil }) })
	n := NewNotification(NotificationOptions{Title: "Export finished"})
	if err := n.Show(); !errors.Is(err, ErrNotificationsDenied) {
		t.Errorf("Show = %v", err)
	}
	notifications.Lock()
	_, kept := notifications.byID[n.id]
	notifications.Unlock()
	if kept {
		t.Error("the notification is kept")
	}
}

func TestParseColor(t *testing.T) {
	rgba := func(r, g, b, a uint8) platform.Color { return platform.Color{R: r, G: g, B: b, A: a} }
	tests := map[string]platform.Color{
		"#fff":                  rgba(255, 255, 255, 255),
		"#11223344":             rgba(0x11, 0x22, 0x33, 0x44),
		"#1e1e1e":               rgba(0x1e, 0x1e, 0x1e, 255),
		"rgb(10, 20, 30)":       rgba(10, 20, 30, 255),
		"rgba(10,20,30,0.5)":    rgba(10, 20, 30, 128),
		"rgb(100% 0% 0% / 50%)": rgba(255, 0, 0, 128),
		"transparent":           {},
	}
	for in, want := range tests {
		got, err := parseColor(in)
		if err != nil || got != want {
			t.Errorf("parseColor(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "#12", "red", "rgb(1,2)", "#zzzzzz"} {
		if _, err := parseColor(bad); err == nil {
			t.Errorf("parseColor(%q) should fail", bad)
		}
	}
}

func TestParseBackground(t *testing.T) {
	white, black := platform.Color{R: 255, G: 255, B: 255, A: 255}, platform.Color{A: 255}
	tests := map[string]background{
		"#fff":                                 {white, white},
		"light-dark(#fff, #000)":               {white, black},
		" Light-Dark( #FFF,rgb(0, 0, 0) ) ":    {white, black},
		"light-dark(rgb(255 255 255), #000f)":  {white, black},
		"light-dark(transparent, transparent)": {},
	}
	for in, want := range tests {
		if got, err := parseBackground(in); err != nil || got != want {
			t.Errorf("parseBackground(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "light-dark(#fff)", "light-dark(#fff, #000", "light-dark(#fff, #000, #111)", "light-dark(#fff, red)", "light-dark()"} {
		if _, err := parseBackground(bad); err == nil {
			t.Errorf("parseBackground(%q) should fail", bad)
		}
	}
}

// TestSingleInstanceIdentifier checks that an app with another identifier
// but the same name, as the development app of mygo dev that calls SetName,
// does not hand over to the running one.
func TestSingleInstanceIdentifier(t *testing.T) {
	if !App.RequestSingleInstanceLock() {
		t.Fatal("first instance did not get the lock")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "MYGO_TEST_SECOND_INSTANCE=1", "MYGO_TEST_IDENTIFIER=com.example.mygotest.dev")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "locked" {
		t.Fatalf("an app with another identifier printed %q", out)
	}
}

func TestSingleInstance(t *testing.T) {
	if !App.RequestSingleInstanceLock() {
		t.Fatal("first instance did not get the lock")
	}
	got := make(chan []string, 1)
	off := App.OnSecondInstance(func(args []string, wd string) {
		if wd == "" {
			t.Error("working directory not forwarded")
		}
		// The window it focuses can take the focus on Wayland.
		if fb.ActivationToken != "mygo-test-token" {
			t.Errorf("activation token = %q", fb.ActivationToken)
		}
		got <- args
	})
	defer off()
	urls := make(chan string, 1)
	defer App.OnOpenURL(func(u string) { urls <- u })()
	// The main thread reads the schemes when it delivers forwarded arguments.
	schemes := packageURLSchemes
	onMain(func() { packageURLSchemes = "mygo-test" })
	defer onMain(func() { packageURLSchemes = schemes })
	cmd := exec.Command(os.Args[0], "-test.run=^$", "--from-second", "instance", "mygo-test://open?x=1")
	cmd.Env = append(os.Environ(), "MYGO_TEST_SECOND_INSTANCE=1", "XDG_ACTIVATION_TOKEN=mygo-test-token")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	// It lets the first instance bring its windows to the front (Windows).
	if strings.TrimSpace(string(out)) != "forwarded\npassed the foreground on" {
		t.Fatalf("second instance printed %q", out)
	}
	select {
	case args := <-got:
		if !slices.Contains(args, "--from-second") || !slices.Contains(args, "instance") {
			t.Errorf("forwarded args = %q", args)
		}
	case <-time.After(wait.Guard):
		t.Fatal("OnSecondInstance not called")
	}
	// Deep links started a second instance: the first one opens them.
	select {
	case u := <-urls:
		if u != "mygo-test://open?x=1" {
			t.Errorf("OnOpenURL got %q", u)
		}
	case <-time.After(wait.Guard):
		t.Fatal("OnOpenURL not called for the forwarded URL")
	}
}
