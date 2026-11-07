//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"testing"
	"time"

	"github.com/caelyreth/evil-mygo/ui"
)

func TestArchiveWaitsForStartupAction(t *testing.T) {
	launcher := newArchiveLauncher()
	t.Cleanup(launcher.close)
	tt := ui.NewTester(launcher.view, 960, 700)
	tt.Advance(time.Hour)
	if launcher.archive != nil || tt.FramePending() {
		t.Fatal("startup loaded the archive or kept requesting idle frames")
	}
	tt.Key(0, ui.KeyRight)
	if launcher.archive != nil {
		t.Fatal("scene navigation bypassed the startup action")
	}
	tt.Click("Open archive")
	if launcher.archive == nil {
		t.Fatal("the startup button did not create the archive")
	}
	archive := launcher.archive
	if archive.controller.clock.elapsed != 0 || archive.controller.interactive {
		t.Fatal("waiting at startup skipped the entrance animation")
	}
	launcher.open()
	if launcher.archive != archive {
		t.Fatal("repeated startup actions replaced the live scene")
	}
}
