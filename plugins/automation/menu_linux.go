//go:build linux && (amd64 || arm64) && !mygo_noinspector

package automation

import (
	"github.com/caelyreth/evil-mygo"
	"github.com/caelyreth/evil-mygo/internal/linux"
)

// chooseMenuItem chooses an item of the window's menu bar, which shows
// the application menu, by the labels along its path. Main thread only.
func chooseMenuItem(w *mygo.Window, path []string) error {
	return linux.TestActivateMenuItem(w.NativeHandle(), path...)
}
