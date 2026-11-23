//go:build !mygo_noinspector && !js

package mygo

import (
	"testing"

	"github.com/egoist/mygo/internal/fake"
	"github.com/egoist/mygo/ui"
)

func TestContentInspector(t *testing.T) {
	var width float32
	view := func(c *ui.Context) {
		width, _ = c.Size()
		ui.Text(c, "Hello")
	}
	// Development builds turn DevTools on: Toggle Developer Tools opens
	// the inspector of native UI beside the content, not a web inspector.
	w, _, s := contentWindow(t, view)
	onMain(func() {
		performRole(RoleToggleDevTools, w)
		s.Frame()
	})
	if width != 150 {
		t.Errorf("beside the inspector, the content is %v wide", width)
	}
	onMain(func() {
		performRole(RoleToggleDevTools, w)
		s.Frame()
	})
	if width != 300 {
		t.Errorf("with the inspector closed, the content is %v wide", width)
	}

	off := NewWindow(WindowOptions{Width: 300, Height: 200, Content: ui.View(view), DevTools: DevToolsDisabled})
	t.Cleanup(off.Destroy)
	onMain(func() {
		performRole(RoleToggleDevTools, off)
		off.conn.Surface.(*fake.Surface).Frame()
	})
	if width != 300 {
		t.Errorf("without DevTools, the role opened the inspector: the content is %v wide", width)
	}
}
