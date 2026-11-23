//go:build !js

package mygo

import (
	"testing"

	"github.com/egoist/mygo/internal/surface"
)

// activeContent is a Content that records what its window tells of its
// active state.
type activeContent struct {
	conn    *surface.Conn
	changes []bool
}

func (c *activeContent) AttachContent(conn *surface.Conn) {
	c.conn = conn
	conn.ActiveChanged = func() { c.changes = append(c.changes, conn.Active()) }
}

// active reports whether w is drawn as active, once the update its focus
// events posted ran: a call on the main thread queues behind it.
func active(w *Window) bool { return onMainValue(func() bool { return w.active }) }

// TestWindowActiveFollowsFocus checks that a window is drawn as active
// while it has the keyboard, and that its native UI hears the changes.
func TestWindowActiveFollowsFocus(t *testing.T) {
	content := &activeContent{}
	a, fa := testWindow(t, WindowOptions{Content: content})
	b, fb := testWindow(t, WindowOptions{})
	if !active(a) || !active(b) || !content.conn.Active() {
		t.Fatal("windows that never had the keyboard start inactive")
	}
	onMain(fa.Focus)
	if !active(a) || !active(b) {
		t.Errorf("one window focused: active %v and %v, the other never focused", active(a), active(b))
	}
	onMain(func() { fa.Blur(); fb.Focus() })
	if active(a) || !active(b) {
		t.Errorf("the other focused: active %v and %v", active(a), active(b))
	}
	// The app goes to the background.
	onMain(fb.Blur)
	if active(a) || active(b) {
		t.Errorf("none focused: active %v and %v", active(a), active(b))
	}
	onMain(fa.Focus)
	if !active(a) {
		t.Error("focused again, not active")
	}
	if got := onMainValue(func() []bool { return content.changes }); len(got) != 2 || got[0] || !got[1] {
		t.Errorf("the content heard %v, want [false true]", got)
	}
}

// TestWindowActiveFlyout checks that a window stays active while its
// flyout has the keyboard, as a window's menus and popovers leave it, and
// that a flyout without the keyboard is as active as its parent.
func TestWindowActiveFlyout(t *testing.T) {
	parent, fp := testWindow(t, WindowOptions{})
	onMain(fp.Focus)
	fl, ff := testFlyout(t, FlyoutOptions{Parent: parent, Focusable: true, Width: 100, Height: 100})
	tip, _ := testFlyout(t, FlyoutOptions{Parent: parent, Width: 100, Height: 20})
	// The parent loses the keyboard to its flyout within one event.
	onMain(func() { fp.Blur(); ff.Focus() })
	if !active(parent) || !active(fl) || !active(tip) {
		t.Errorf("a flyout has the keyboard: parent active %v, flyout %v, the other %v", active(parent), active(fl), active(tip))
	}
	// Another app comes in front.
	onMain(ff.Blur)
	if active(parent) || active(fl) || active(tip) {
		t.Errorf("none focused: parent active %v, flyout %v, the other %v", active(parent), active(fl), active(tip))
	}
}

// TestWindowActiveKept checks the windows drawn active whatever has the
// keyboard: panels, flyouts of trays, those SetActiveAppearance keeps
// active, and a window that never had the keyboard; and that one shown
// without it is not.
func TestWindowActiveKept(t *testing.T) {
	panel, fpanel := testWindow(t, WindowOptions{Panel: true})
	kept, fk := testWindow(t, WindowOptions{})
	shown, _ := testWindow(t, WindowOptions{Hidden: true})
	other, fo := testWindow(t, WindowOptions{})
	onMain(func() { fpanel.Focus(); fpanel.Blur(); fk.Focus(); fk.Blur() })
	kept.SetActiveAppearance(true)
	shown.ShowInactive()
	onMain(fo.Focus)
	if !active(panel) || !active(kept) || !fk.ActiveAppearance() {
		t.Errorf("active: panel %v, kept %v (backend %v)", active(panel), active(kept), fk.ActiveAppearance())
	}
	if active(shown) || !active(other) {
		t.Errorf("active: shown without the keyboard %v, focused %v", active(shown), active(other))
	}
	kept.SetActiveAppearance(false)
	if active(kept) {
		t.Error("active after SetActiveAppearance(false)")
	}
}
