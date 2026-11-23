//go:build !js

package mygo

import "testing"

// TestPanelOptions gives a panel's backend a window above others, out of
// the taskbar, that cannot minimize, maximize or go full screen.
func TestPanelOptions(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{Panel: true})
	o := fw.Opts
	if !w.IsPanel() || !o.Panel || !o.AlwaysOnTop || !o.SkipTaskbar || o.Minimizable || o.Maximizable || o.Fullscreenable {
		t.Errorf("panel: IsPanel %v, options %+v", w.IsPanel(), o)
	}
	if !o.Focusable || !o.Resizable || !o.Closable {
		t.Errorf("a panel lost abilities it keeps: %+v", o)
	}
	w, fw = testWindow(t, WindowOptions{})
	if w.IsPanel() || fw.Opts.Panel {
		t.Error("a window without Panel is a panel")
	}
}
