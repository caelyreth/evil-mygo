//go:build darwin && !ios

package darwin

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestDarwinOverlaysLifecycleAndPulse(t *testing.T) {
	runtime.LockOSThread()
	t.Cleanup(runtime.UnlockOSThread)
	load()

	var s *surface
	withPool(func() {
		view := send(send(class("NSView"), "alloc"), "init")
		msgSetRect(view, sel("setFrame:"), NSRect{Origin: NSPoint{0, 0}, Size: NSSize{300, 200}})
		send(view, "setWantsLayer:", 1)
		s = &surface{w: &window{}, view: view}
	})
	t.Cleanup(func() {
		withPool(func() {
			s.destroyOverlays()
			release(s.view)
		})
	})

	withPool(func() {
		if !s.CompositorOverlaysSupported() {
			t.Fatal("CompositorOverlaysSupported() = false, want true")
		}
		rootLayer := send(s.view, "layer")
		if rootLayer == 0 {
			t.Fatal("NSView backing layer is nil")
		}
		baseCount := sendInt(send(rootLayer, "sublayers"), "count")

		place := platform.OverlayPlacement{
			ID:           42,
			Frame:        platform.RectF{X: 20, Y: 30, W: 2, H: 18},
			Clip:         platform.RectF{X: 20, Y: 30, W: 2, H: 18},
			CornerRadius: 1,
			Color:        platform.Color{R: 15, G: 110, B: 230, A: 255},
			Opacity:      1,
			PulseStart:   time.Now(),
			PulseDelay:   450 * time.Millisecond,
			PulsePeriod:  1200 * time.Millisecond,
			PulseMin:     0,
			PulseMax:     1,
		}
		s.PlaceOverlays([]platform.OverlayPlacement{place})

		if got := sendInt(send(rootLayer, "sublayers"), "count"); got != baseCount+1 {
			t.Fatalf("sublayers count = %d, want %d", got, baseCount+1)
		}
		ol := s.overlays[42]
		if ol == nil || ol.clip == 0 || ol.fill == 0 {
			t.Fatalf("missing overlay layers: %+v", ol)
		}
		if z := msgFloat(ol.clip, sel("zPosition")); z != overlayZPosition {
			t.Fatalf("clip zPosition = %v, want %v", z, overlayZPosition)
		}
		if !sendBool(ol.clip, "masksToBounds") {
			t.Fatal("clip layer masksToBounds = false, want true")
		}
		anim := send(ol.fill, "animationForKey:", uintptr(nsString(overlayPulseAnimKey)))
		if anim == 0 {
			t.Fatal("missing Core Animation pulse on fill layer")
		}
		if !sendBool(anim, "autoreverses") {
			t.Fatal("CABasicAnimation autoreverses = false, want true")
		}

		// Geometry-only updates preserve the existing layer pair and animation.
		prevFill := ol.fill
		place.Frame.X = 44
		place.Clip.X = 44
		s.PlaceOverlays([]platform.OverlayPlacement{place})
		if s.overlays[42].fill != prevFill {
			t.Fatal("geometry update replaced fill layer instead of reusing it")
		}
		if send(ol.fill, "animationForKey:", uintptr(nsString(overlayPulseAnimKey))) == 0 {
			t.Fatal("geometry update dropped pulse animation")
		}

		// Empty places remove overlay sublayers cleanly.
		s.PlaceOverlays(nil)
		if len(s.overlays) != 0 {
			t.Fatalf("after empty PlaceOverlays: len(s.overlays) = %d, want 0", len(s.overlays))
		}
		if got := sendInt(send(rootLayer, "sublayers"), "count"); got != baseCount {
			t.Fatalf("after empty PlaceOverlays: sublayers count = %d, want %d", got, baseCount)
		}

		// Re-add and verify destroyOverlays cleans up everything.
		s.PlaceOverlays([]platform.OverlayPlacement{place})
		if len(s.overlays) != 1 {
			t.Fatalf("after re-add: len(s.overlays) = %d, want 1", len(s.overlays))
		}
		s.destroyOverlays()
		if len(s.overlays) != 0 {
			t.Fatalf("after destroyOverlays: len(s.overlays) = %d, want 0", len(s.overlays))
		}
		if got := sendInt(send(rootLayer, "sublayers"), "count"); got != baseCount {
			t.Fatalf("after destroyOverlays: sublayers count = %d, want %d", got, baseCount)
		}
	})
}
