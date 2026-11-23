//go:build !js

package mygo

import (
	"testing"
	"time"

	"github.com/egoist/mygo/internal/wait"
)

func TestPower(t *testing.T) {
	var got []string
	record := func(event string) func() { return func() { got = append(got, event) } }
	offs := []func(){
		Power.OnSuspend(record("suspend")),
		Power.OnResume(record("resume")),
		Power.OnLockScreen(record("lock")),
		Power.OnUnlockScreen(record("unlock")),
	}
	defer func() {
		for _, off := range offs {
			off()
		}
	}()
	wait.Until(t, "the backend asked to watch power events", func() bool { return onMainValue(func() bool { return fb.Watching }) })
	onMain(func() {
		for _, e := range []string{"suspend", "resume", "lock-screen", "unlock-screen", "unknown"} {
			fb.EmitPower(e)
		}
	})
	if want := "suspend resume lock unlock"; join(got) != want {
		t.Errorf("events = %q, want %q", join(got), want)
	}

	release := Power.KeepAwake("testing", true)
	if n := onMainValue(func() int { return fb.Awake }); n != 1 {
		t.Errorf("awake = %d", n)
	}
	release()
	release() // once
	if n := onMainValue(func() int { return fb.Awake }); n != 0 {
		t.Errorf("awake after release = %d", n)
	}

	release = Power.KeepRunning("serving")
	if n := onMainValue(func() int { return fb.Running }); n != 1 {
		t.Errorf("running = %d", n)
	}
	if n := onMainValue(func() int { return fb.Awake }); n != 0 {
		t.Errorf("KeepRunning kept the computer awake (%d)", n)
	}
	release()
	release() // once
	if n := onMainValue(func() int { return fb.Running }); n != 0 {
		t.Errorf("running after release = %d", n)
	}
	if Power.IsOnBattery() || Power.IdleTime() != 42*time.Second {
		t.Error("IsOnBattery or IdleTime")
	}
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += " "
		}
		out += x
	}
	return out
}

func TestDockMenuAndRelaunch(t *testing.T) {
	m := NewMenu([]*MenuItem{{Label: "New Window", ID: "new"}})
	App.Dock.SetMenu(m)
	if App.Dock.Menu() != m {
		t.Error("Dock.Menu")
	}
	dm := onMainValue(func() any { return fb.DockMenu })
	if dm == nil || len(fb.DockMenu.Items) != 1 || fb.DockMenu.Items[0].Label != "New Window" {
		t.Errorf("dock menu = %+v", dm)
	}
	App.Dock.SetMenu(nil)
	if onMainValue(func() bool { return fb.DockMenu != nil }) {
		t.Error("SetMenu(nil) kept the menu")
	}

	// A relaunch whose quit is canceled does nothing. Before the windows
	// close, it lets the app it starts take the foreground, which the
	// app gives up as they close (Windows).
	canceled := make(chan struct{}, 1)
	allowed := onMainValue(func() int { return fb.ForegroundAllowed })
	off := App.OnBeforeQuit(func(e *QuitEvent) {
		if fb.ForegroundAllowed != allowed+1 {
			t.Errorf("the foreground was passed on %d times before the quit", fb.ForegroundAllowed-allowed)
		}
		e.PreventDefault()
		canceled <- struct{}{}
	})
	defer off()
	App.Relaunch("--after-restart")
	select {
	case <-canceled:
	case <-time.After(wait.Guard):
		t.Fatal("Relaunch did not start quitting")
	}
	if onMainValue(func() bool { return App.relaunch || App.relaunchArgs != nil }) {
		t.Error("a canceled relaunch is still pending, or its arguments")
	}
}
