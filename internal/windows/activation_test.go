//go:build windows && (amd64 || arm64)

package windows

import (
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

type recordingWindowHandler struct {
	platform.WindowHandler
	events []platform.SurfaceEvent
}

func (h *recordingWindowHandler) Focused() {}
func (h *recordingWindowHandler) Blurred() {}

func (h *recordingWindowHandler) SurfaceEvent(ev platform.SurfaceEvent) bool {
	h.events = append(h.events, ev)
	return true
}

func TestMouseActivateConsumeActivationClick(t *testing.T) {
	b := &Backend{
		windows: map[uintptr]*window{},
	}
	for _, enabled := range []bool{false, true} {
		w := &window{
			b:       b,
			opts:    &platform.WindowOptions{ConsumeActivationClick: enabled},
			surface: &surface{},
		}
		w.surface.w = w

		// Client area activation press while inactive.
		res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(wmLButtonDown)<<16)
		if enabled {
			if !handled || res != maActivateAndEat {
				t.Fatalf("enabled=true, inactive, htClient: got (%d, %v), want (%d, true)", res, handled, maActivateAndEat)
			}
			if w.surface.ateUp != wmLButtonUp {
				t.Fatalf("surface.ateUp = %#x, want %#x", w.surface.ateUp, wmLButtonUp)
			}
		} else if handled {
			t.Fatalf("enabled=false, inactive, htClient: expected unhandled, got (%d, %v)", res, handled)
		}

		// Once active, WM_MOUSEACTIVATE on the client area must not eat subsequent clicks.
		w.active = true
		w.surface.ateUp = 0
		if res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(wmLButtonDown)<<16); handled {
			t.Fatalf("enabled=%v, active, htClient: expected unhandled, got (%d, %v)", enabled, res, handled)
		}
		w.active = false

		// Non-client areas (title bar, window controls, resize edges) keep default activation.
		for _, ht := range []uintptr{htCaption, htMinButton, htMaxButton, htClose, htTop, htTopLeft} {
			w.surface.ateUp = 0
			res, handled := w.message(wmMouseActivate, 0, ht|uintptr(wmLButtonDown)<<16)
			if handled {
				t.Fatalf("enabled=%v, ht=%d: expected unhandled, got (%d, %v)", enabled, ht, res, handled)
			}
			if w.surface.ateUp != 0 {
				t.Fatalf("enabled=%v, ht=%d: unexpected ateUp=%#x", enabled, ht, w.surface.ateUp)
			}
		}
	}
}

func TestSurfaceEatActivationRelease(t *testing.T) {
	h := &recordingWindowHandler{}
	b := &Backend{
		windows:  map[uintptr]*window{},
		surfaces: map[uintptr]*surface{},
	}
	w := &window{b: b, h: h, opts: &platform.WindowOptions{ConsumeActivationClick: true}}
	s := &surface{w: w, hwnd: 1234}
	w.surface = s

	for _, tc := range []struct {
		down uint16
		up   uint32
	}{
		{wmLButtonDown, wmLButtonUp},
		{wmRButtonDown, wmRButtonUp},
		{wmMButtonDown, wmMButtonUp},
		{wmXButtonDown, wmXButtonUp},
	} {
		h.events = nil
		if res, handled := w.message(wmMouseActivate, 0, htClient|uintptr(tc.down)<<16); !handled || res != maActivateAndEat {
			t.Fatalf("down=%#x: got (%d, %v), want (%d, true)", tc.down, res, handled, maActivateAndEat)
		}
		if res, handled := s.message(s.hwnd, tc.up, 0, 0); !handled || res != 0 {
			t.Fatalf("up=%#x: expected eaten (0, true), got (%d, %v)", tc.up, res, handled)
		}
		if len(h.events) != 0 {
			t.Fatalf("up=%#x: expected no surface events, got %+v", tc.up, h.events)
		}
		if s.ateUp != 0 {
			t.Fatalf("up=%#x: expected ateUp cleared, got %#x", tc.up, s.ateUp)
		}
	}

	// A stale ateUp (e.g. pointer released outside the window) is cleared by the next press.
	s.eatActivationRelease(wmLButtonDown)
	s.message(s.hwnd, wmLButtonDown, 1, 0)
	if s.ateUp != 0 {
		t.Fatalf("expected ateUp cleared on wmLButtonDown, got %#x", s.ateUp)
	}
	s.message(s.hwnd, wmLButtonUp, 0, 0)
	if len(h.events) != 2 || h.events[0].Kind != platform.PointerDown || h.events[1].Kind != platform.PointerUp {
		t.Fatalf("expected normal down+up after cleared ateUp, got %+v", h.events)
	}
}

