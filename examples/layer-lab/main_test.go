package main

import (
	"bytes"
	"image"
	"testing"
	"time"

	"github.com/caelyreth/evil-mygo/ui"
	"github.com/caelyreth/evil-mygo/ui/shader"
)

func TestNativeControlsSurviveEffects(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1240, 820)
	for i, mode := range modes {
		if err := tt.Click(mode.name); err != nil {
			t.Fatal(err)
		}
		if a.mode != i {
			t.Fatalf("selected mode %d, want %d", a.mode, i)
		}
		if err := tt.Click("Toggle playback"); err != nil {
			t.Fatal(err)
		}
		if a.playing != (i%2 == 0) {
			t.Fatalf("playback did not toggle in %s", mode.name)
		}
		if err := tt.Click("Save track"); err != nil {
			t.Fatal(err)
		}
		if a.saved != (i%2 == 0) {
			t.Fatalf("save did not toggle in %s", mode.name)
		}
	}
	if err := tt.Click("Track title"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("A real native field")
	if a.title != "A real native field" {
		t.Fatalf("track title = %q, want the edited text", a.title)
	}
}

func TestPointerAndReset(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1240, 820)
	r, ok := tt.Find("Native specimen")
	if !ok {
		t.Fatal("no native specimen")
	}
	tt.Move(r.X+r.W*0.2, r.Y+r.H*0.2)
	if abs(a.center.X-0.2) > 0.01 || abs(a.center.Y-0.2) > 0.01 {
		t.Fatalf("lens at %+v after moving to 20%%, want (0.2, 0.2)", a.center)
	}
	if err := tt.Click("Follow pointer"); err != nil {
		t.Fatal(err)
	}
	pinned := a.center
	tt.Move(r.X+r.W*0.8, r.Y+r.H*0.8)
	if a.center != pinned {
		t.Fatal("a pinned lens followed the pointer")
	}
	a.mode, a.strength, a.radius, a.dispersion = 3, 0.2, 0.9, 0.1
	a.animate, a.bypass = false, true
	tt.Frame()
	if err := tt.Click("Reset experiment"); err != nil {
		t.Fatal(err)
	}
	if a.mode != 0 || a.strength != 0.75 || a.radius != 0.58 || a.dispersion != 0.5 || !a.follow || !a.animate || a.bypass {
		t.Fatal("reset did not restore the optical controls")
	}
}

func TestPreferencesAndIdle(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1000, 740)
	for _, prefs := range []ui.Preferences{{ReduceMotion: true}, {ReduceTransparency: true}} {
		tt.SetPreferences(prefs)
		if a.clock.running || tt.FramePending() {
			t.Fatal("an accessibility preference left an animation running")
		}
	}
	tt.SetPreferences(ui.Preferences{})
	if err := tt.Click("Animate light"); err != nil {
		t.Fatal(err)
	}
	if a.clock.running || tt.FramePending() {
		t.Fatal("the paused view keeps requesting frames")
	}
	for _, size := range [][2]int{{1240, 820}, {1000, 740}, {1440, 960}} {
		tt.SetSize(size[0], size[1])
		stage, _ := tt.Find("Native specimen")
		for _, name := range []string{"Track title", "Track position", "Toggle playback", "Save track"} {
			r, ok := tt.Find(name)
			if !ok || r.W <= 0 || r.H <= 0 || r.X < stage.X || r.Y < stage.Y || r.X+r.W > stage.X+stage.W || r.Y+r.H > stage.Y+stage.H {
				t.Errorf("%s at %+v outside specimen %+v in %v", name, r, stage, size)
			}
		}
	}
}

func TestClockPauseAndResume(t *testing.T) {
	var c animationClock
	now := time.Unix(100, 0)
	c.setRunning(now, true)
	if got := c.at(now.Add(3 * time.Second)); got != 3 {
		t.Fatalf("clock = %v, want 3", got)
	}
	c.setRunning(now.Add(3*time.Second), false)
	if got := c.at(now.Add(50 * time.Second)); got != 3 {
		t.Fatalf("paused clock = %v, want 3", got)
	}
	c.setRunning(now.Add(50*time.Second), true)
	if got := c.at(now.Add(51 * time.Second)); got != 4 {
		t.Fatalf("resumed clock = %v, want 4", got)
	}
}

func fixture(mode int, strength, scale float32, translucent bool) *image.RGBA {
	return ui.Render(func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		e := ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			for i := range 12 {
				color := ui.RGB(uint8(20+i*17), uint8(220-i*12), uint8(40+i*13))
				if translucent {
					color.A = 160
				}
				p.Fill(ui.Rect{X: r.X + float32(i)*6, Y: r.Y, W: 6, H: r.H}, color, 0)
			}
			p.Fill(ui.Rect{X: 10, Y: 17, W: 43, H: 7}, ui.RGBA(255, 248, 235, 0.8), 0)
		})
		if mode >= 0 {
			e.Effect(optics.Effect(shader.Options{Uniforms: map[string]any{
				"Mode": mode, "Time": 0.7, "Center": [2]float32{35, 23},
				"Strength": strength, "Radius": 0.6, "Dispersion": 0.7,
			}}))
		}
	}, 72, 48, scale)
}

func TestShaderModesAndPremultiplication(t *testing.T) {
	for _, translucent := range []bool{false, true} {
		original := fixture(-1, 0, 1, translucent)
		for mode := range modes {
			got := fixture(mode, 0.85, 1, translucent)
			if bytes.Equal(got.Pix, original.Pix) {
				t.Errorf("%s did not change the source, translucent=%v", modes[mode].name, translucent)
			}
			for i := 0; i < len(got.Pix); i += 4 {
				if p := got.Pix[i : i+4]; p[0] > p[3] || p[1] > p[3] || p[2] > p[3] {
					t.Fatalf("%s returned non-premultiplied pixel %v", modes[mode].name, p)
				}
			}
		}
	}
}

func TestZeroStrengthIsOriginalAtEveryScale(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		original := fixture(-1, 0, scale, false)
		for mode := range modes {
			if got := fixture(mode, 0, scale, false); !bytes.Equal(got.Pix, original.Pix) {
				t.Errorf("%s at strength 0 changed the source at scale %v", modes[mode].name, scale)
			}
		}
	}
}

func abs(v float32) float32 { return max(v, -v) }
