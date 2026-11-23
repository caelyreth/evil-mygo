//go:build !js

package e2e

import (
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestWindowActive switches the keyboard between two native UI windows,
// and checks that each is drawn active while it has it, and that a window
// stays active while its focusable flyout has it.
func TestWindowActive(t *testing.T) {
	var firstActive, secondActive atomic.Bool
	w := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, X: 40, Y: 40, Content: ui.View(func(c *ui.Context) {
		firstActive.Store(c.WindowActive())
		ui.Text(c, "first")
	})})
	native := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, X: 400, Y: 40, Content: ui.View(func(c *ui.Context) {
		secondActive.Store(c.WindowActive())
		ui.Text(c, "second")
	})})

	native.Focus()
	eventually(t, "the second window focused", func() bool { return mygo.FocusedWindow() == native })
	eventually(t, "the first window drawn inactive", func() bool { return !firstActive.Load() })
	eventually(t, "the second window drawn active", secondActive.Load)

	w.Focus()
	eventually(t, "the first window focused", func() bool { return mygo.FocusedWindow() == w })
	eventually(t, "the first window drawn active", firstActive.Load)
	eventually(t, "the second window drawn inactive", func() bool { return !secondActive.Load() })

	// The first window keeps its active appearance while its flyout has the keyboard.
	fl := mygo.NewFlyout(mygo.FlyoutOptions{Parent: w, Anchor: mygo.Rectangle{X: 10, Y: 10}, Width: 200, Height: 120, Focusable: true,
		Content: ui.View(func(c *ui.Context) { ui.Text(c, "flyout") })})
	defer fl.Destroy()
	eventually(t, "the flyout focused", func() bool { return mygo.FocusedWindow() == fl })
	if !firstActive.Load() {
		t.Error("the parent window drawn inactive while its flyout has the keyboard")
	}
	if secondActive.Load() {
		t.Error("the other window drawn active while another window's flyout has the keyboard")
	}
}

// TestMaterialFollowsActive checks the state of macOS's materials: a
// window's follows whether it is active, as AppKit's sidebars do, while
// panels, flyouts and a window SetActiveAppearance keeps active keep
// theirs active, floating over whatever app is in front.
func TestMaterialFollowsActive(t *testing.T) {
	const follows, active = 0, 1 // NSVisualEffectState
	w := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, Vibrancy: mygo.VibrancySidebar, Transparent: true})
	state, ok := materialState(w)
	if !ok {
		t.Skip("the state of materials is macOS's")
	}
	if state != follows {
		t.Errorf("a window's material in state %d, want %d", state, follows)
	}
	panel := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, Panel: true, Vibrancy: mygo.VibrancyHUD, Transparent: true})
	fl := mygo.NewFlyout(mygo.FlyoutOptions{Parent: w, Anchor: mygo.Rectangle{X: 10, Y: 10}, Width: 200, Height: 120, Vibrancy: mygo.VibrancyPopover})
	defer fl.Destroy()
	for name, x := range map[string]*mygo.Window{"panel": panel, "flyout": fl} {
		if state, _ := materialState(x); state != active {
			t.Errorf("a %s's material in state %d, want %d", name, state, active)
		}
	}
	w.SetActiveAppearance(true)
	if state, _ := materialState(w); state != active {
		t.Errorf("kept active: state %d, want %d", state, active)
	}
	w.SetActiveAppearance(false)
	if state, _ := materialState(w); state != follows {
		t.Errorf("no longer kept active: state %d, want %d", state, follows)
	}
}
