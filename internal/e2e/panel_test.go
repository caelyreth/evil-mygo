//go:build !js

package e2e

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// TestPanelNativeUI closes a panel of native UI on an Escape its content
// leaves.
func TestPanelNativeUI(t *testing.T) {
	panel := newWindow(t, mygo.WindowOptions{Width: 300, Height: 100, Panel: true, Content: ui.View(func(c *ui.Context) {
		ui.Text(c, "Panel")
	})})
	eventually(t, "the panel focused", panel.IsFocused)
	time.Sleep(200 * time.Millisecond)
	if !pressEscape(panel) {
		t.Skip("keys not available on this platform")
	}
	eventually(t, "the panel closed on Escape", panel.IsDestroyed)
}
