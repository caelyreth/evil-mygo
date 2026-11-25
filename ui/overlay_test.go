package ui

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/surface"
)

func newOverlayTester(view func(c *context)) *repaintTester {
	tt := newRepaintTester(view)
	tt.h.overlaySupport = true
	tt.rt.changed()
	tt.frame(0)
	return tt
}

func countFillOps(ops []scene.Op) int {
	n := 0
	for i := range ops {
		if ops[i].Kind == scene.OpFill {
			n++
		}
	}
	return n
}

func hasFillColor(ops []scene.Op, c Color) bool {
	want := c.scene()
	for i := range ops {
		if ops[i].Kind == scene.OpFill && ops[i].Color == want {
			return true
		}
	}
	return false
}

func TestOverlayFillUnobstructed(t *testing.T) {
	start := time.Unix(1000, 0)
	caretColor := RGB(20, 120, 240)
	pulse := Pulse{
		Start:    start,
		Delay:    450 * time.Millisecond,
		Period:   1200 * time.Millisecond,
		Min:      0,
		Max:      1,
		Interval: time.Second / 30,
	}
	tt := newOverlayTester(func(c *context) {
		coreBox(c).Size(100, 40).Draw(func(p *Painter, r Rect) {
			p.OverlayFill(Rect{X: r.X + 10, Y: r.Y + 5, W: 2, H: 18}, caretColor, 1, pulse)
		})
	})

	// Only the root background fill (Ops[0]) should exist; no fill op for the overlay.
	if got := countFillOps(tt.rt.scene.Ops); got != 1 || hasFillColor(tt.rt.scene.Ops, caretColor) {
		t.Fatalf("unobstructed OverlayFill emitted %d OpFill ops (has overlay fill=%v), want 1 root fill only",
			got, hasFillColor(tt.rt.scene.Ops, caretColor))
	}
	if !tt.rt.repaintAt.IsZero() || !tt.rt.repaintDue.IsZero() {
		t.Fatalf("unobstructed OverlayFill scheduled wakeup: repaintAt=%v repaintDue=%v", tt.rt.repaintAt, tt.rt.repaintDue)
	}
	if len(tt.h.overlays) != 1 {
		t.Fatalf("got %d compositor overlays, want 1", len(tt.h.overlays))
	}
	got := tt.h.overlays[0]
	wantFrame := platform.RectF{X: 10, Y: 5, W: 2, H: 18}
	if got.Frame != wantFrame || got.Clip != wantFrame {
		t.Fatalf("overlay Frame=%+v Clip=%+v, want %+v", got.Frame, got.Clip, wantFrame)
	}
	if got.CornerRadius != 1 {
		t.Fatalf("CornerRadius = %v, want 1", got.CornerRadius)
	}
	if wantCol := (platform.Color{R: 20, G: 120, B: 240, A: 255}); got.Color != wantCol {
		t.Fatalf("Color = %+v, want %+v", got.Color, wantCol)
	}
	if !got.PulseStart.Equal(start) || got.PulseDelay != pulse.Delay || got.PulsePeriod != pulse.Period || got.PulseMin != 0 || got.PulseMax != 1 {
		t.Fatalf("unexpected pulse params: %+v", got)
	}

	_, more := tt.frame(500 * time.Millisecond)
	if more || !tt.rt.repaintAt.IsZero() || !tt.rt.repaintDue.IsZero() {
		t.Fatalf("idle frame after unobstructed overlay: more=%v repaintAt=%v repaintDue=%v",
			more, tt.rt.repaintAt, tt.rt.repaintDue)
	}
	if len(tt.h.overlays) != 1 || countFillOps(tt.rt.scene.Ops) != 1 {
		t.Fatalf("after unrequested refresh: overlays=%d fills=%d, want 1 and 1",
			len(tt.h.overlays), countFillOps(tt.rt.scene.Ops))
	}
}

type fakeOverlaySurface struct {
	platform.Surface
	supported bool
	calls     int
	got       []platform.OverlayPlacement
}

func (f *fakeOverlaySurface) CompositorOverlaysSupported() bool { return f.supported }
func (f *fakeOverlaySurface) PlaceOverlays(places []platform.OverlayPlacement) {
	f.calls++
	f.got = append(f.got[:0], places...)
}

func TestWindowHostPlaceOverlaysDedup(t *testing.T) {
	surf := &fakeOverlaySurface{supported: true}
	h := &windowHost{conn: &surface.Conn{Surface: surf}}
	if !h.compositorOverlays() {
		t.Fatal("compositorOverlays() = false, want true")
	}

	t0 := time.Unix(2000, 0)
	places := []platform.OverlayPlacement{{
		ID:           7,
		Frame:        platform.RectF{X: 10, Y: 20, W: 2, H: 16},
		Clip:         platform.RectF{X: 10, Y: 20, W: 2, H: 16},
		CornerRadius: 1,
		Color:        platform.Color{R: 10, G: 20, B: 30, A: 255},
		Opacity:      1,
		PulseStart:   t0,
		PulseDelay:   450 * time.Millisecond,
		PulsePeriod:  1200 * time.Millisecond,
		PulseMin:     0,
		PulseMax:     1,
	}}

	h.placeOverlays(places)
	if surf.calls != 1 {
		t.Fatalf("after first placeOverlays: calls = %d, want 1", surf.calls)
	}

	// Identical placement slice must be deduplicated without calling PlaceOverlays.
	dup := append([]platform.OverlayPlacement(nil), places...)
	h.placeOverlays(dup)
	if surf.calls != 1 {
		t.Fatalf("duplicate placeOverlays: calls = %d, want 1", surf.calls)
	}

	// Geometry change triggers a new call.
	places[0].Frame.X = 11
	h.placeOverlays(places)
	if surf.calls != 2 {
		t.Fatalf("after frame change: calls = %d, want 2", surf.calls)
	}

	// PulseStart change triggers a new call.
	places[0].PulseStart = t0.Add(time.Second)
	h.placeOverlays(places)
	if surf.calls != 3 {
		t.Fatalf("after PulseStart change: calls = %d, want 3", surf.calls)
	}

	// Clearing overlays triggers once, then deduplicates.
	h.placeOverlays(nil)
	h.placeOverlays(nil)
	if surf.calls != 4 || len(surf.got) != 0 {
		t.Fatalf("after clear: calls=%d len=%d, want 4 and 0", surf.calls, len(surf.got))
	}
}

func TestOverlayFillOcclusionAndRecovery(t *testing.T) {
	occlude := false
	ovColor := RGB(0, 120, 255)
	tt := newOverlayTester(func(c *context) {
		coreBox(c).Size(100, 40).Draw(func(p *Painter, r Rect) {
			p.OverlayFill(Rect{X: r.X + 10, Y: r.Y + 10, W: 4, H: 20}, ovColor, 1, Pulse{
				Start:    p.Now(),
				Delay:    100 * time.Millisecond,
				Period:   time.Second,
				Max:      1,
				Interval: time.Second / 30,
			})
			if occlude {
				p.Fill(Rect{X: r.X + 8, Y: r.Y + 12, W: 10, H: 6}, RGB(255, 0, 0), 0)
			}
		})
	})

	if len(tt.h.overlays) != 1 || countFillOps(tt.rt.scene.Ops) != 1 || hasFillColor(tt.rt.scene.Ops, ovColor) || !tt.rt.repaintAt.IsZero() {
		t.Fatalf("initial unobstructed: overlays=%d fills=%d hasOvFill=%v repaintAt=%v",
			len(tt.h.overlays), countFillOps(tt.rt.scene.Ops), hasFillColor(tt.rt.scene.Ops, ovColor), tt.rt.repaintAt)
	}

	occlude = true
	tt.rt.changed()
	tt.frame(20 * time.Millisecond)
	if len(tt.h.overlays) != 0 {
		t.Fatalf("while occluded: overlays=%d, want 0", len(tt.h.overlays))
	}
	if got := countFillOps(tt.rt.scene.Ops); got != 3 || !hasFillColor(tt.rt.scene.Ops, ovColor) {
		t.Fatalf("while occluded: OpFill count = %d (hasOvFill=%v), want 3 (root + overlay fallback + occluder)",
			got, hasFillColor(tt.rt.scene.Ops, ovColor))
	}
	if tt.rt.repaintAt.IsZero() {
		t.Fatal("while occluded: expected scheduled repaintAt for pulsing fallback")
	}

	occlude = false
	tt.rt.changed()
	tt.frame(20 * time.Millisecond)
	if len(tt.h.overlays) != 1 || countFillOps(tt.rt.scene.Ops) != 1 || hasFillColor(tt.rt.scene.Ops, ovColor) || !tt.rt.repaintAt.IsZero() {
		t.Fatalf("after un-occlude recovery: overlays=%d fills=%d hasOvFill=%v repaintAt=%v",
			len(tt.h.overlays), countFillOps(tt.rt.scene.Ops), hasFillColor(tt.rt.scene.Ops, ovColor), tt.rt.repaintAt)
	}
}

func TestOverlayFillFallbacks(t *testing.T) {
	pulse := Pulse{Period: time.Second, Max: 1, Interval: time.Second / 30}
	ovColor := RGB(10, 20, 30)
	cases := []struct {
		name           string
		overlaySupport bool
		view           func(c *context)
	}{
		{
			name:           "unsupported",
			overlaySupport: false,
			view: func(c *context) {
				coreBox(c).Size(80, 40).Draw(func(p *Painter, r Rect) {
					p.OverlayFill(Rect{X: r.X + 5, Y: r.Y + 5, W: 2, H: 16}, ovColor, 1, pulse)
				})
			},
		},
		{
			name:           "rounded-clip",
			overlaySupport: true,
			view: func(c *context) {
				coreBox(c).Size(80, 40).Radius(8).Clip().Children(func() {
					coreBox(c).Size(80, 40).Draw(func(p *Painter, r Rect) {
						p.OverlayFill(Rect{X: r.X + 5, Y: r.Y + 5, W: 2, H: 16}, ovColor, 1, pulse)
					})
				})
			},
		},
		{
			name:           "transform",
			overlaySupport: true,
			view: func(c *context) {
				coreBox(c).Size(80, 40).Draw(func(p *Painter, r Rect) {
					p.Transform(Translation(5, 5), r.X+20, r.Y+20, func() {
						p.OverlayFill(Rect{X: r.X + 5, Y: r.Y + 5, W: 2, H: 16}, ovColor, 1, pulse)
					})
				})
			},
		},
		{
			name:           "fractional-opacity",
			overlaySupport: true,
			view: func(c *context) {
				coreBox(c).Size(80, 40).Opacity(0.5).Children(func() {
					coreBox(c).Size(80, 40).Draw(func(p *Painter, r Rect) {
						p.OverlayFill(Rect{X: r.X + 5, Y: r.Y + 5, W: 2, H: 16}, ovColor, 1, pulse)
					})
				})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tt := newRepaintTester(tc.view)
			tt.h.overlaySupport = tc.overlaySupport
			tt.rt.changed()
			tt.frame(0)
			if len(tt.h.overlays) != 0 {
				t.Fatalf("%s: got %d compositor overlays, want 0", tc.name, len(tt.h.overlays))
			}
			if got := countFillOps(tt.rt.scene.Ops); got != 2 {
				t.Fatalf("%s: OpFill count = %d, want 2 (root + fallback fill)", tc.name, got)
			}
			if tt.rt.repaintAt.IsZero() {
				t.Fatalf("%s: expected fallback repaintAt wakeup", tc.name)
			}
		})
	}
}

func TestOverlayRepaintPartsSurvival(t *testing.T) {
	start := time.Unix(1000, 0)
	occludeInPart := false
	part3Color := RGB(30, 40, 220)
	part4Color := RGB(90, 90, 90)
	part3Pulse := Pulse{
		Start:  start,
		Period: time.Second,
		Max:    1,
	}
	tt := newOverlayTester(func(c *context) {
		// All 4 parts fit vertically within the 200x100 viewport so none are culled.
		// Part 1: static compositor overlay that does not ask for animation frames.
		coreBox(c).Size(40, 20).Draw(func(p *Painter, r Rect) {
			p.OverlayFill(Rect{X: r.X + 4, Y: r.Y + 2, W: 2, H: 14}, RGB(10, 200, 40), 1, Pulse{
				Start:  start,
				Period: time.Second,
				Max:    1,
			})
		})
		// Part 2: animated non-overlapping part that drives repaintParts.
		coreBox(c).Size(40, 20).Draw(func(p *Painter, r Rect) {
			p.AnimationFrame()
			dx := float32((p.Now().UnixMilli() / 16) % 10)
			p.Fill(Rect{X: r.X + 2 + dx, Y: r.Y + 2, W: 4, H: 4}, RGB(200, 30, 30), 0)
		})
		// Part 3: animated part containing an overlay that toggles occlusion via repaintParts.
		coreBox(c).Size(40, 20).Draw(func(p *Painter, r Rect) {
			p.AnimationFrame()
			p.OverlayFill(Rect{X: r.X + 4, Y: r.Y + 2, W: 3, H: 14}, part3Color, 1, part3Pulse)
			if occludeInPart {
				p.Fill(Rect{X: r.X + 3, Y: r.Y + 4, W: 10, H: 6}, RGB(240, 200, 0), 0)
			}
		})
		// Part 4: trailing part after Part 3 to verify op-range shifting across splices.
		coreBox(c).Size(40, 20).Draw(func(p *Painter, r Rect) {
			p.Fill(Rect{X: r.X + 1, Y: r.Y + 1, W: 5, H: 5}, part4Color, 0)
		})
	})

	if len(tt.h.overlays) != 2 {
		t.Fatalf("initial overlays = %d, want 2", len(tt.h.overlays))
	}
	wantPart1 := tt.h.overlays[0]
	if got := countFillOps(tt.rt.scene.Ops); got != 3 || !hasFillColor(tt.rt.scene.Ops, part4Color) {
		t.Fatalf("initial OpFill count = %d (part4=%v), want 3 (root + Part 2 + Part 4)",
			got, hasFillColor(tt.rt.scene.Ops, part4Color))
	}

	// Unobstructed repaintParts frames preserve Part 1's untouched overlay identically.
	for i := range 2 {
		built, _ := tt.frame(16 * time.Millisecond)
		if built {
			t.Fatalf("frame %d rebuilt whole view instead of repaintParts", i)
		}
	}
	if tt.rt.partPaints < 2 {
		t.Fatalf("partPaints = %d, want >= 2", tt.rt.partPaints)
	}
	if len(tt.h.overlays) != 2 || !sameOverlayPlacement(tt.h.overlays[0], wantPart1) {
		t.Fatalf("Part 1 overlay changed during repaintParts: got=%+v want=%+v", tt.h.overlays, wantPart1)
	}

	// Splicing Part 3's overlay into scene during repaintParts keeps Part 1 alive and Part 4 intact.
	occludeInPart = true
	built, _ := tt.frame(16 * time.Millisecond)
	if built {
		t.Fatal("occlusion toggle inside part rebuilt whole view")
	}
	if len(tt.h.overlays) != 1 || !sameOverlayPlacement(tt.h.overlays[0], wantPart1) {
		t.Fatalf("while Part 3 occluded: overlays=%+v, want only Part 1", tt.h.overlays)
	}
	wantPart3Fill := part3Color.Alpha(pulseOpacity(part3Pulse, tt.clock))
	if got := countFillOps(tt.rt.scene.Ops); got != 5 || !hasFillColor(tt.rt.scene.Ops, wantPart3Fill) || !hasFillColor(tt.rt.scene.Ops, part4Color) {
		t.Fatalf("while Part 3 occluded: OpFill count = %d (part3=%v part4=%v), want 5",
			got, hasFillColor(tt.rt.scene.Ops, wantPart3Fill), hasFillColor(tt.rt.scene.Ops, part4Color))
	}

	// Un-occluding Part 3 inside repaintParts unsplices cleanly back to 2 compositor overlays and 3 fills.
	occludeInPart = false
	built, _ = tt.frame(16 * time.Millisecond)
	if built {
		t.Fatal("un-occlusion inside part rebuilt whole view")
	}
	if len(tt.h.overlays) != 2 || !sameOverlayPlacement(tt.h.overlays[0], wantPart1) {
		t.Fatalf("after Part 3 un-occluded: overlays=%+v, want 2 with Part 1 intact", tt.h.overlays)
	}
	if got := countFillOps(tt.rt.scene.Ops); got != 3 || hasFillColor(tt.rt.scene.Ops, part3Color) || !hasFillColor(tt.rt.scene.Ops, part4Color) {
		t.Fatalf("after Part 3 un-occluded: OpFill count = %d (part3=%v part4=%v), want 3",
			got, hasFillColor(tt.rt.scene.Ops, part3Color), hasFillColor(tt.rt.scene.Ops, part4Color))
	}
}

func TestPulseMath(t *testing.T) {
	t0 := time.Unix(1000, 0)
	p := Pulse{
		Start:    t0,
		Delay:    400 * time.Millisecond,
		Period:   1200 * time.Millisecond,
		Min:      0.2,
		Max:      0.8,
		Interval: 25 * time.Millisecond,
	}

	if got := pulseOpacity(p, t0.Add(150*time.Millisecond)); got != 0.8 {
		t.Fatalf("during Delay: opacity = %v, want 0.8", got)
	}
	if wait, ok := pulseNextWake(p, t0.Add(150*time.Millisecond)); !ok || wait != 250*time.Millisecond {
		t.Fatalf("during Delay: wake = (%v, %v), want (250ms, true)", wait, ok)
	}

	for _, tc := range []struct {
		at   time.Duration
		want float32
	}{
		{400 * time.Millisecond, 0.8},
		{400*time.Millisecond + 300*time.Millisecond, 0.5},
		{400*time.Millisecond + 600*time.Millisecond, 0.2},
		{400*time.Millisecond + 1200*time.Millisecond, 0.8},
	} {
		got := pulseOpacity(p, t0.Add(tc.at))
		if math.Abs(float64(got-tc.want)) > 1e-5 {
			t.Errorf("at %v: opacity = %v, want %v", tc.at, got, tc.want)
		}
		if wait, ok := pulseNextWake(p, t0.Add(tc.at)); !ok || wait != p.Interval {
			t.Errorf("at %v: wake = (%v, %v), want (%v, true)", tc.at, wait, ok, p.Interval)
		}
	}

	static := Pulse{Period: 0, Min: 0, Max: 0}
	if got := pulseOpacity(static, t0); got != 1 {
		t.Fatalf("zero Period default opacity = %v, want 1", got)
	}
	if wait, ok := pulseNextWake(static, t0); ok || wait != 0 {
		t.Fatalf("zero Period wake = (%v, %v), want (0, false)", wait, ok)
	}
}
