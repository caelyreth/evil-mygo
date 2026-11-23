//go:build windows && (amd64 || arm64)

package e2e

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo"
	win "github.com/egoist/mygo/internal/windows"
	"github.com/egoist/mygo/ui"
)

// TestConsumeActivationClickInactiveInput checks wheel, hover, and file drops
// over an inactive native UI window when ConsumeActivationClick is enabled on Windows.
func TestConsumeActivationClickInactiveInput(t *testing.T) {
	file := filepath.Join(t.TempDir(), "drop.txt")
	if err := os.WriteFile(file, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	focusOther := func(t *testing.T) *mygo.Window {
		t.Helper()
		var frames atomic.Int32
		other := newWindow(t, mygo.WindowOptions{
			Width: 200, Height: 150, X: 400, Y: 40,
			Content: ui.View(func(c *ui.Context) {
				frames.Add(1)
				ui.Text(c, "Other")
			}),
		})
		eventually(t, "the other native frame built", func() bool { return frames.Load() > 0 })
		other.Focus()
		requireForeground(t, other)
		eventually(t, "the other window focused", other.IsFocused)
		// Park the pointer on the other window so a previous test's cursor
		// position cannot trigger hover on the target window before deactivation.
		mygo.RunOnMain(func() { win.TestInputMove(other.NativeHandle(), 50, 50) })
		return other
	}

	t.Run("native", func(t *testing.T) {
		var frames, hovered, dropped atomic.Int32
		var scroll ui.ScrollState
		var scrollY atomic.Value
		scrollY.Store(float32(0))
		w := newWindow(t, mygo.WindowOptions{
			Width: 300, Height: 200, X: 40, Y: 40,
			ConsumeActivationClick: true,
			Content: ui.View(func(c *ui.Context) {
				frames.Add(1)
				ui.Scroll(c).Fill().TrackScroll(&scroll).Children(func() {
					b := ui.Box(c).Width(300).Height(600)
					if b.Hovered() {
						hovered.Store(1)
					}
					if files := b.DroppedFiles(); len(files) > 0 {
						dropped.Store(int32(len(files)))
					}
				})
				scrollY.Store(scroll.Y)
			}),
		})
		eventually(t, "the native frame built", func() bool { return frames.Load() > 0 })
		focusOther(t)
		hovered.Store(0)

		eventually(t, "hover over inactive native UI", func() bool {
			mygo.RunOnMain(func() { win.TestInputMove(w.NativeHandle(), 100, 100) })
			return hovered.Load() == 1
		})
		mygo.RunOnMain(func() { win.TestInputWheel(w.NativeHandle(), 100, 100) })
		eventually(t, "wheel scroll over inactive native UI", func() bool {
			return scrollY.Load().(float32) > 0
		})
		droppedFile, supported := dragFiles(w, 100, 100, []string{file})
		if !supported || !droppedFile {
			t.Fatalf("inactive native drop: supported=%v dropped=%v", supported, droppedFile)
		}
		eventually(t, "drop over inactive native UI", func() bool { return dropped.Load() == 1 })
		if w.IsFocused() {
			t.Fatal("hover, wheel, or drop activated the window")
		}
	})
}
