package ui

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestEditChordReachesHandlerAsCommand checks that a handler of its own
// keys that leaves an edit chord gets it as the command, as the Edit menu
// gives it on macOS, and that one taking the key gets no command.
func TestEditChordReachesHandlerAsCommand(t *testing.T) {
	for _, tc := range []struct {
		name  string
		takes bool
		want  string
	}{
		{"leaves the key", false, "selectAll,redo,redo,undo"},
		{"takes the key", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			tt := coreNewTester(func(c *context) {
				coreBox(c).Size(100, 100).Focusable().Label("Canvas").AutoFocus().HandleInput(func(ev InputEvent) bool {
					if ev.Kind == InputCommand {
						got = append(got, ev.Text)
						return true
					}
					return ev.Kind == InputKeyDown && tc.takes
				})
			}, 200, 200)
			cmd := platform.Modifiers(Cmd)
			for _, k := range []struct {
				mods platform.Modifiers
				key  platform.Key
			}{{cmd, platform.KeyA}, {cmd | platform.ModShift, platform.KeyZ}, {cmd, platform.KeyY}, {cmd, platform.KeyZ}, {0, platform.KeyB}} {
				tt.send(platform.SurfaceEvent{Kind: platform.KeyPressed, Mods: k.mods, Key: k.key})
				tt.send(platform.SurfaceEvent{Kind: platform.KeyReleased, Mods: k.mods, Key: k.key})
			}
			if strings.Join(got, ",") != tc.want {
				t.Fatalf("commands %q, want %q", got, tc.want)
			}
		})
	}
}
