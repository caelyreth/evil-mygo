package ui

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/internal/surface"
)

// testFlyout is a flyout a test's window opens, its view in a Tester.
type testFlyout struct {
	tt              *Tester
	spec            surface.FlyoutSpec
	shown, focused  bool
	closed, invalid bool
	onClose         []func()
}

func (f *testFlyout) Show(focus bool) { f.shown, f.focused = true, focus }
func (f *testFlyout) Focus()          { f.focused = true }
func (f *testFlyout) Invalidate()     { f.invalid = true }
func (f *testFlyout) OnClose(fn func()) {
	f.onClose = append(f.onClose, fn)
}
func (f *testFlyout) Close() {
	if f.closed {
		return
	}
	f.closed = true
	for _, fn := range f.onClose {
		fn()
	}
}

// flyingMenus opens the flyouts of tt's window as Testers of their own,
// which it returns as they open.
func flyingMenus(tt *Tester) *[]*testFlyout {
	var opened []*testFlyout
	tt.h.flyouts = func(spec surface.FlyoutSpec, content surface.Content) surface.Flyout {
		f := &testFlyout{tt: coreNewTester(content.(*Content).view, 240, 320), spec: spec}
		opened = append(opened, f)
		return f
	}
	return &opened
}

// open returns the flyouts opened that have not closed.
func open(fs []*testFlyout) []*testFlyout {
	return slices.DeleteFunc(slices.Clone(fs), func(f *testFlyout) bool { return f.closed })
}

func TestFlyoutMenus(t *testing.T) {
	var chosen []string
	view := func(c *context) {
		c.SetMenuPresentation(FlyoutMenus)
		coreColumn(c).Fill().Padding(20).Children(func() {
			coreRow(c).Padding(8).Children(func() { coreText(c, "Report.pdf") }).ContextMenu(func(m *Menu) {
				if m.Item("Open").Chosen() {
					chosen = append(chosen, "Open")
				}
				m.Submenu("Move to", func(m *Menu) {
					if m.Item("Desktop").Chosen() {
						chosen = append(chosen, "Desktop")
					}
				})
			})
		})
	}
	tt := coreNewTester(view, 400, 300)
	flyouts := flyingMenus(tt)
	tt.RightClick("Report.pdf")
	tt.Frame()
	if tt.h.menu != nil || tt.HasText("Open") {
		t.Fatalf("system menu %v, drawn in the window %v; want a flyout", tt.h.menu != nil, tt.HasText("Open"))
	}
	fs := open(*flyouts)
	if len(fs) != 1 || !fs[0].shown || !fs[0].focused || fs[0].spec.Parent != nil {
		t.Fatalf("flyouts %+v; want the menu's, shown with the keyboard", fs)
	}
	menu := fs[0]
	menu.tt.Frame()
	if !menu.tt.HasText("Open") || !menu.tt.HasText("Move to") {
		t.Fatalf("the menu's flyout shows %q", menu.tt.Texts())
	}
	// The keys open the submenu in a flyout of the menu's, after its row.
	menu.tt.Key(0, KeyDown)
	menu.tt.Key(0, KeyDown)
	menu.tt.Key(0, KeyRight)
	tt.Frame()
	fs = open(*flyouts)
	if len(fs) != 2 || fs[1].spec.Parent != menu || fs[1].spec.Placement != "right-start" || !fs[1].focused {
		t.Fatalf("after Right, flyouts %+v; want the submenu's, of the menu's, with the keyboard", fs)
	}
	sub := fs[1]
	sub.tt.Frame()
	if !sub.tt.HasText("Desktop") {
		t.Fatalf("the submenu's flyout shows %q", sub.tt.Texts())
	}
	// Left closes it.
	sub.tt.Key(0, KeyLeft)
	tt.Frame()
	if !sub.closed || menu.closed {
		t.Fatalf("after Left, submenu closed %v, menu closed %v", sub.closed, menu.closed)
	}
	// A click on an item in a flyout chooses it, and closes them all.
	menu.tt.Key(0, KeyRight)
	tt.Frame()
	sub = open(*flyouts)[1]
	sub.tt.Frame()
	if err := sub.tt.Click("Desktop"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !slices.Equal(chosen, []string{"Desktop"}) || len(open(*flyouts)) != 0 {
		t.Fatalf("chose %q, flyouts open %d", chosen, len(open(*flyouts)))
	}
	// Dismissed by the user, the menu closes without a choice.
	tt.RightClick("Report.pdf")
	tt.Frame()
	menu = open(*flyouts)[0]
	menu.Close()
	tt.Frame()
	if tt.rt.menu.drawn != nil || len(chosen) != 1 {
		t.Fatalf("after its flyout closed, the menu shows %v, chose %q", tt.rt.menu.drawn != nil, chosen)
	}
}

// Where the window opens no flyout, FlyoutMenus draws in it.
func TestFlyoutMenusInTheWindow(t *testing.T) {
	var chosen string
	view := func(c *context) {
		c.SetMenuPresentation(FlyoutMenus)
		coreColumn(c).Fill().Padding(20).Children(func() {
			coreRow(c).Padding(8).Children(func() { coreText(c, "Report.pdf") }).ContextMenu(func(m *Menu) {
				if m.Item("Open").Chosen() {
					chosen = "Open"
				}
			})
		})
	}
	tt := coreNewTester(view, 400, 300)
	tt.RightClick("Report.pdf")
	wantDrawn(t, tt, "Open")
	if err := tt.Click("Open"); err != nil || chosen != "Open" {
		t.Fatalf("chose %q: %v", chosen, err)
	}
}

// A palette shows in a drawn menu as a row of swatches in place of its
// item, which the keys take as one item.
func TestDrawnMenuPalette(t *testing.T) {
	colors := []struct {
		name string
		c    Color
	}{{"Red", RGB(220, 40, 40)}, {"Green", RGB(40, 160, 60)}, {"Blue", RGB(40, 90, 220)}}
	tag, chosen := "Green", ""
	view := drawingMenus(func(c *context) {
		coreColumn(c).Fill().Padding(20).Children(func() {
			coreRow(c).Padding(8).Children(func() { coreText(c, "Note") }).ContextMenu(func(m *Menu) {
				m.Item("Rename")
				m.Palette("Color", func(m *Menu) {
					for _, k := range colors {
						if m.Item(k.name).Swatch(k.c).Radio(tag == k.name).Chosen() {
							chosen = k.name
						}
					}
				})
			})
		})
	})
	tt := coreNewTester(view, 400, 300)
	tt.RightClick("Note")
	if _, ok := tt.AccessByRole(RoleMenuItem, "Color"); ok {
		t.Fatal("the palette shows as an item opening a submenu, not as a row of swatches")
	}
	if _, ok := tt.AccessByRole(RoleGroup, "Color"); !ok {
		t.Fatal("no group named Color")
	}
	for _, k := range colors {
		if sw, ok := tt.AccessByRole(RoleMenuItemRadio, k.name); !ok || sw.Checked != (k.name == tag) {
			t.Fatalf("swatch %s: shown %v, checked %v", k.name, ok, sw.Checked)
		}
	}
	// Down, Down: onto the row, at its checked swatch; Right: the next.
	tt.Key(0, KeyDown)
	tt.Key(0, KeyDown)
	tt.Key(0, KeyRight)
	tt.Key(0, KeyEnter)
	tt.Frame()
	if chosen != "Blue" {
		t.Fatalf("chose %q, want Blue", chosen)
	}
}

// SurfaceHighlight highlights a row in the theme's SurfaceHover, its text
// keeping its color.
func TestDrawnMenuSurfaceHighlight(t *testing.T) {
	view := func(c *context) {
		c.SetMenuPresentation(PopoverMenus)
		c.SetMenuStyle(PopoverMenuOptions{Highlight: SurfaceHighlight})
		coreColumn(c).Fill().Padding(20).Children(func() {
			coreRow(c).Padding(8).Children(func() { coreText(c, "Note") }).ContextMenu(func(m *Menu) {
				m.Item("Rename")
			})
		})
	}
	tt := coreNewTester(view, 400, 300)
	tt.RightClick("Note")
	tt.Key(0, KeyDown)
	tt.Frame()
	th := tt.rt.c.theme
	var row *node
	var walk func(e *node)
	walk = func(e *node) {
		for ch := e.first; ch != nil; ch = ch.next {
			if ch.role == RoleMenuItem && ch.highlighted {
				row = ch
			}
			walk(ch)
		}
	}
	walk(tt.rt.c.overlay)
	if row == nil {
		t.Fatal("no row highlighted")
	}
	if row.bg != th.SurfaceHover || row.ts.color != th.Text {
		t.Fatalf("highlighted row: background %v, text %v; want %v, %v", row.bg, row.ts.color, th.SurfaceHover, th.Text)
	}
}
