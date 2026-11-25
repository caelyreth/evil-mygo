//go:build !js

package mygo

import (
	"math"
	"testing"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/ui"
)

func TestFitContentDIPs(t *testing.T) {
	f := &fitContent{min: platform.Size{Width: 100, Height: 40}, max: platform.Size{Width: 300, Height: 200}}
	for _, c := range []struct {
		w, h, zoom float64
		want       platform.Size
	}{
		{120, 80, 1, platform.Size{Width: 120, Height: 80}},
		{120.5, 80.25, 1, platform.Size{Width: 121, Height: 81}}, // fractions take a pixel
		{120.004, 80, 1, platform.Size{Width: 120, Height: 80}},  // but not rounding errors
		{60.25, 45, 2, platform.Size{Width: 121, Height: 90}},    // CSS pixels of a zoomed page
		{150, 100, 0, platform.Size{Width: 150, Height: 100}},    // no zoom is 1
		{10, 10, 1, platform.Size{Width: 100, Height: 40}},       // the minimum
		{500, 400, 1, platform.Size{Width: 300, Height: 200}},    // the maximum
		{0, 0, 1, platform.Size{Width: 100, Height: 40}},
	} {
		if got, ok := f.fit(contentSize{Width: c.w, Height: c.h}, c.zoom, 0); !ok || got != c.want {
			t.Errorf("fit(%v, %v, zoom %v) = %v, %v; want %v", c.w, c.h, c.zoom, got, ok, c.want)
		}
	}
	unbounded := &fitContent{}
	if got, _ := unbounded.fit(contentSize{Height: 1e300}, 1, 0); got != (platform.Size{Width: 1, Height: maxFitDIPs}) {
		t.Errorf("unbounded fit of an empty, huge page = %v", got)
	}
	for _, bad := range [][2]float64{{-1, 10}, {10, -0.5}} {
		if got, ok := f.fit(contentSize{Width: bad[0], Height: bad[1]}, 1, 0); ok {
			t.Errorf("fit(%v) = %v, want no size", bad, got)
		}
	}
}

// The room of the scrollbars counts where the content needs them: a
// vertical scrollbar where the content is taller than the window may be,
// not while the window is only shorter than the content for a moment.
func TestFitContentScrollbars(t *testing.T) {
	f := &fitContent{max: platform.Size{Width: 300, Height: 200}}
	for _, c := range []struct {
		what      string
		c         contentSize
		maxHeight int
		want      platform.Size
	}{
		{"a vertical scrollbar for a moment", contentSize{Width: 120, Height: 150, Scrollbar: 17}, 0, platform.Size{Width: 120, Height: 150}},
		{"taller than the maximum", contentSize{Width: 120, Height: 500, Scrollbar: 17}, 0, platform.Size{Width: 137, Height: 200}},
		{"taller than the work area", contentSize{Width: 120, Height: 150, Scrollbar: 17}, 100, platform.Size{Width: 137, Height: 100}},
		{"overlay scrollbars", contentSize{Width: 120, Height: 500}, 0, platform.Size{Width: 120, Height: 200}},
		{"wider than the maximum", contentSize{Width: 300, Height: 60, HorizontalScrollbar: 17}, 0, platform.Size{Width: 300, Height: 77}},
		{"zoomed", contentSize{Width: 150, Height: 60, HorizontalScrollbar: 8.5}, 0, platform.Size{Width: 300, Height: 137}},
		{"no room", contentSize{Width: 120, Height: 500, Scrollbar: -1, HorizontalScrollbar: math.NaN()}, 0, platform.Size{Width: 120, Height: 200}},
	} {
		zoom := 1.0
		if c.what == "zoomed" {
			zoom = 2
		}
		if got, ok := f.fit(c.c, zoom, c.maxHeight); !ok || got != c.want {
			t.Errorf("%s: fit(%+v) = %v, %v; want %v", c.what, c.c, got, ok, c.want)
		}
	}
}

// A flyout of native UI with FitContent takes the size its content
// measures, within its limits and the work area, and follows it as it
// changes, placed against its anchor at each size: two frames, the first
// laid out at the size the flyout has, the next at the size it took.
func TestFlyoutFitNativeContent(t *testing.T) {
	// The fake display's work area is 1440x875 at 0,25: the anchor is 100
	// DIPs above its bottom.
	parent, _ := testWindow(t, WindowOptions{X: 100, Y: 600, Width: 400, Height: 300})
	rows, width := 2, float32(100)
	fl, fw := testFlyout(t, FlyoutOptions{
		Parent:     parent,
		Anchor:     Rectangle{Y: 200},
		Width:      150,
		Height:     120,
		FitContent: true,
		MinWidth:   100,
		MinHeight:  40,
		MaxWidth:   300,
		MaxHeight:  250,
		Content: ui.View(func(c *ui.Context) {
			ui.Column(c).Fill().Padding(10).Gap(5).Children(func() {
				for i := 0; i < rows; i++ {
					ui.Box(c).Size(width, 20)
				}
			})
		}),
	})
	s := fw.FakeSurface()
	resizes := 0
	// settle draws the frames the flyout asks for, as the display would,
	// and runs the resize each posts; a surface tells its content it
	// resized, which asks for the next frame.
	settle := func() {
		t.Helper()
		for i := 0; ; i++ {
			if i == 10 {
				t.Fatal("the flyout keeps resizing")
			}
			before := fl.Bounds()
			drew := false
			onMain(func() { drew = s.Frame() })
			onMain(func() {})
			if b := fl.Bounds(); b.Width == before.Width && b.Height == before.Height {
				if !drew {
					return
				}
				continue
			}
			resizes++
			onMain(func() { s.Send(platform.SurfaceEvent{Kind: platform.SurfaceResize}) })
		}
	}
	bounds := func(what string, x, y, w, h int) {
		t.Helper()
		if b := fl.Bounds(); b != (Rectangle{X: x, Y: y, Width: w, Height: h}) {
			t.Errorf("%s: flyout %+v, want %dx%d at %d,%d", what, b, w, h, x, y)
		}
	}
	update := func(fn func()) {
		t.Helper()
		fl.Update(fn)
		onMain(func() {})
		settle()
	}
	// It fits before it shows: its first frame is at its content's size,
	// below the anchor, and resizes nothing.
	bounds("before its first frame", 100, 800, 120, 65)
	settle()
	bounds("its content's size, below the anchor", 100, 800, 120, 65)
	if resizes != 0 {
		t.Errorf("%d resizes for the first frame", resizes)
	}

	// It follows the content, flipping above the anchor where the work
	// area has no room below.
	update(func() { rows = 12 })
	bounds("taller, above the anchor", 100, 800-250, 120, 250) // 12 rows: 315 DIPs, the most 250
	update(func() { rows, width = 1, 20 })
	bounds("smaller, below again", 100, 800, 100, 40)
	update(func() { width = 500 })
	bounds("wider than the most", 100, 800, 300, 40)

	// The same content resizes nothing, and a size SetSize gives holds
	// until the content's changes.
	resizes = 0
	fl.Invalidate()
	onMain(func() {})
	settle()
	if resizes != 0 {
		t.Errorf("%d resizes for the same content", resizes)
	}
	fl.SetSize(220, 90)
	fl.Invalidate()
	onMain(func() {})
	settle()
	bounds("the same content after SetSize", 100, 800, 220, 90)
	update(func() { width = 180 })
	bounds("other content after SetSize", 100, 800, 200, 40)

	// Without limits, the work area bounds it.
	free, ffw := testFlyout(t, FlyoutOptions{
		Parent:     parent,
		FitContent: true,
		Content:    ui.View(func(c *ui.Context) { ui.Box(c).Size(2000, 2000) }),
	})
	s = ffw.FakeSurface()
	fl = free
	settle()
	if b := free.Bounds(); b.Width != 1440 || b.Height != 875 {
		t.Errorf("huge content: flyout %+v, want the work area's 1440x875", b)
	}
}

// A hidden flyout of native UI with FitContent draws no frames, as a
// hidden window does: content that changed meanwhile lays out as the
// flyout shows, which shows at the size it takes, not at the size it hid
// at with a resize after its first frame.
func TestFlyoutFitNativeContentHidden(t *testing.T) {
	parent, _ := testWindow(t, WindowOptions{X: 100, Y: 100, Width: 400, Height: 300})
	rows, width, builds := 2, float32(100), 0
	fl, fw := testFlyout(t, FlyoutOptions{
		Parent:     parent,
		Anchor:     Rectangle{Y: 200},
		Width:      150,
		Height:     120,
		FitContent: true,
		MinWidth:   100,
		MinHeight:  40,
		Hidden:     true,
		Content: ui.View(func(c *ui.Context) {
			builds++
			ui.Column(c).Fill().Padding(10).Gap(5).Children(func() {
				for i := 0; i < rows; i++ {
					ui.Box(c).Size(width, 20)
				}
			})
		}),
	})
	s := fw.FakeSurface()
	resizes := 0
	// settle draws the frames the flyout asks for, as the display would
	// while it shows, and runs the resizes they post.
	settle := func() {
		t.Helper()
		for i := 0; ; i++ {
			if i == 10 {
				t.Fatal("the flyout keeps resizing")
			}
			before := fl.Bounds()
			drew := false
			onMain(func() { drew = s.Frame() })
			onMain(func() {})
			if b := fl.Bounds(); b.Width == before.Width && b.Height == before.Height {
				if !drew {
					return
				}
				continue
			}
			resizes++
			onMain(func() { s.Send(platform.SurfaceEvent{Kind: platform.SurfaceResize}) })
		}
	}
	// The size the platform has the flyout at as it shows it.
	var shown platform.Size
	fl.OnShow(func() {
		b := fw.Bounds()
		shown = platform.Size{Width: b.Width, Height: b.Height}
	})
	show := func(what string, showFn func(), w, h int) {
		t.Helper()
		resizes = 0
		showFn()
		if want := (platform.Size{Width: w, Height: h}); shown != want {
			t.Errorf("%s: shown at %v, want %v", what, shown, want)
		}
		settle()
		if b := fl.Bounds(); b.Width != w || b.Height != h || resizes != 0 {
			t.Errorf("%s: %+v after %d resizes once shown, want %dx%d after none", what, b, resizes, w, h)
		}
	}

	// Its first frame draws while hidden, which makes it ready to show.
	settle()
	if b := fl.Bounds(); b.Width != 120 || b.Height != 65 {
		t.Fatalf("hidden, after its first frame: %+v, want 120x65", b)
	}
	// Changed while hidden, before its first Show: 4 rows.
	fl.Update(func() { rows = 4 })
	onMain(func() {})
	show("changed before its first Show", fl.Show, 120, 115)

	// Shown, hidden, changed and shown again.
	fl.Hide()
	fl.Update(func() { rows, width = 1, 20 })
	onMain(func() {})
	show("changed while hidden", fl.ShowInactive, 100, 40)

	// Changed on the main thread, with Invalidate, and shown at once.
	fl.Hide()
	show("invalidated and shown at once", func() {
		onMain(func() {
			rows = 3
			fl.Invalidate()
			fl.Show()
		})
	}, 100, 90)

	// Unchanged while hidden: nothing builds before it shows.
	fl.Hide()
	show("unchanged", func() {
		n := builds
		fl.Show()
		if builds != n {
			t.Errorf("%d builds as a flyout shows again unchanged", builds-n)
		}
	}, 100, 90)
}
