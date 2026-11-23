//go:build !js

package e2e

import (
	"bytes"
	"image/png"
	"math"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/internal/wait"
	"github.com/egoist/mygo/ui"
)

// waitSize waits for size to return width and height.
func waitSize(t *testing.T, what string, _ *mygo.Window, size func() (int, int), width, height int) {
	t.Helper()
	var gotW, gotH int
	if !wait.Poll(func() bool {
		gotW, gotH = size()
		return gotW == width && gotH == height
	}) {
		t.Fatalf("%s: %dx%d never happened (waited %v): %dx%d", what, width, height, wait.Guard, gotW, gotH)
	}
}

// TestFlyoutFitNativeContent sizes a flyout of native UI to its content,
// follows the content as it changes, within the limits, and draws the
// content at the size the flyout took.
func TestFlyoutFitNativeContent(t *testing.T) {
	work := mygo.Screen.PrimaryDisplay().WorkArea
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, X: work.X + 100, Y: work.Y + 100})
	var w, h atomic.Int32
	w.Store(120)
	h.Store(80)
	fl := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:     parent,
		Anchor:     mygo.Rectangle{X: 10, Y: 10},
		Width:      50,
		Height:     50,
		FitContent: true,
		MinWidth:   100,
		MinHeight:  40,
		MaxWidth:   300,
		MaxHeight:  200,
		Content: ui.View(func(c *ui.Context) {
			ui.Box(c).Size(float32(w.Load()), float32(h.Load())).Background(ui.RGB(255, 204, 0))
		}),
	})
	defer fl.Destroy()
	size := func() (int, int) { b := fl.Bounds(); return b.Width, b.Height }
	resize := func(width, height int32) {
		fl.Update(func() { w.Store(width); h.Store(height) })
	}
	waitSize(t, "the content's size", fl, size, 120, 80)
	resize(180, 150)
	waitSize(t, "larger content", fl, size, 180, 150)
	resize(20, 10)
	waitSize(t, "content below the minimum", fl, size, 100, 40)
	resize(500, 400)
	waitSize(t, "content beyond the maximum", fl, size, 300, 200)
	resize(150, 60)
	waitSize(t, "smaller content", fl, size, 150, 60)

	// The last frame was laid out at the flyout's size.
	var gotW, gotH int
	if !wait.Poll(func() bool {
		b, err := fl.CapturePage()
		if err != nil {
			return false
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			return false
		}
		gotW, gotH = img.Bounds().Dx(), img.Bounds().Dy()
		scale := float64(gotW) / 150
		return math.Abs(float64(gotH)-60*scale) <= 1
	}) {
		t.Errorf("the last frame is %dx%d pixels, for a flyout of 150x60 DIPs", gotW, gotH)
	}
}

// TestFlyoutFitNativeContentHidden shows a hidden flyout of native UI
// whose content changed while it was hidden at the size that content
// takes, before its first Show and after it hid again: a hidden window
// draws no frames, so nothing measured the content before it showed.
func TestFlyoutFitNativeContentHidden(t *testing.T) {
	work := mygo.Screen.PrimaryDisplay().WorkArea
	parent := newWindow(t, mygo.WindowOptions{Width: 300, Height: 200, X: work.X + 100, Y: work.Y + 100})
	var w, h atomic.Int32
	w.Store(120)
	h.Store(80)
	fl := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:     parent,
		Anchor:     mygo.Rectangle{X: 10, Y: 10},
		Width:      50,
		Height:     50,
		FitContent: true,
		MinWidth:   100,
		MinHeight:  40,
		MaxWidth:   300,
		MaxHeight:  200,
		Hidden:     true,
		Content: ui.View(func(c *ui.Context) {
			ui.Box(c).Size(float32(w.Load()), float32(h.Load())).Background(ui.RGB(255, 204, 0))
		}),
	})
	defer fl.Destroy()
	size := func() (int, int) { b := fl.Bounds(); return b.Width, b.Height }
	// The flyout's size as it shows, which OnShow hears on the main
	// thread, as Show returns.
	var shownW, shownH int
	fl.OnShow(func() { shownW, shownH = size() })
	showAt := func(what string, show func(), width, height int) {
		t.Helper()
		show()
		if shownW != width || shownH != height {
			t.Errorf("%s: shown at %dx%d, want %dx%d", what, shownW, shownH, width, height)
		}
		waitSize(t, what, fl, size, width, height)
	}
	resize := func(width, height int32) {
		fl.Update(func() { w.Store(width); h.Store(height) })
	}
	// Its first frame draws while it is hidden.
	waitSize(t, "the content's size, hidden", fl, size, 120, 80)
	resize(180, 150)
	showAt("changed before its first Show", fl.Show, 180, 150)

	fl.Hide()
	resize(150, 60)
	showAt("changed while hidden", fl.ShowInactive, 150, 60)
}
