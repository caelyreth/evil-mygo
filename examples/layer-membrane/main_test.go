package main

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestMembraneSurfaceRoundTrip(t *testing.T) {
	s := &membraneSurface{
		cam:       camera{z: 3.65, y: 0.14},
		amplitude: 1,
		waveTime:  1.7,
	}
	s.setViewport(1000, 700)
	for _, uv := range []ui.UV{{U: .2, V: .25}, {U: .5, V: .5}, {U: .8, V: .75}} {
		p, ok := s.SurfaceToScreen(uv)
		if !ok {
			t.Fatalf("UV %+v did not project", uv)
		}
		hit, ok := s.ScreenToSurface(p)
		if !ok || hit.Backface {
			t.Fatalf("UV %+v hit = %+v, %v", uv, hit, ok)
		}
		if math.Abs(float64(hit.UV.U-uv.U)) > 0.012 || math.Abs(float64(hit.UV.V-uv.V)) > 0.012 {
			t.Fatalf("UV %+v round trip = %+v", uv, hit.UV)
		}
	}
	if s.Version() == 0 {
		t.Fatal("surface has no geometry version")
	}
}

func TestMembraneSurfaceRejectsBackfaceProjection(t *testing.T) {
	s := &membraneSurface{
		cam:       camera{z: -3.2, y: 0.14, yaw: float32(math.Pi)},
		amplitude: 1,
	}
	s.setViewport(1000, 700)
	if p, ok := s.SurfaceToScreen(ui.UV{U: .5, V: .5}); ok {
		t.Fatalf("back face projected to %+v", p)
	}
	hit, ok := s.ScreenToSurface(ui.Point{X: 500, Y: 350})
	if !ok || !hit.Backface {
		t.Fatalf("back-face ray = %+v, %v, want a back-face hit", hit, ok)
	}
}

func TestMembraneActionUsesSurfaceUV(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	action := layerBox(t, a, "Send signal")
	x, y, ok := screenPointForLayerBox(a, action)
	if !ok {
		t.Fatalf("action box %+v does not project (view %+v)", action, a.viewBox)
	}
	tt.Press(x, y)
	tt.Release(x, y)
	tt.Frame()
	if a.actions != 1 {
		hit, _ := a.surf.ScreenToSurface(ui.Point{X: x - a.viewBox.X, Y: y - a.viewBox.Y})
		t.Fatalf("surface action count = %d, want 1 (action %+v screen %g,%g hit %+v)", a.actions, action, x, y, hit)
	}
}

func TestMembraneTextInputKeepsNativeState(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	field := layerBox(t, a, "Message")
	x, y, ok := screenPointForLayer(a, field.X+field.W-12, field.Y+field.H/2)
	if !ok {
		t.Fatalf("field box %+v does not project", field)
	}
	tt.Press(x, y)
	tt.Release(x, y)
	tt.Frame()
	if active, _ := tt.TextInputActive(); !active {
		t.Fatal("outer host did not expose the layer text input")
	}
	tt.Type(" Updated")
	tt.Frame()
	if !strings.HasSuffix(a.message, " Updated") {
		t.Fatalf("message = %q, want text typed through the surface", a.message)
	}
}

func TestMembraneCursorFollowsMappedControl(t *testing.T) {
	a := newApp()
	tt := ui.NewTester(a.view, 1440, 900)
	tt.Frame()
	action := layerBox(t, a, "Send signal")
	x, y, ok := screenPointForLayerBox(a, action)
	if !ok {
		t.Fatal("action does not project")
	}
	tt.Move(x, y)
	tt.Frame()
	if got := tt.Cursor(); got != ui.CursorPointer {
		t.Fatalf("cursor over membrane action %+v = %v, want pointer (screen %g,%g)", action, got, x, y)
	}
	tt.Move(10, 10)
	tt.Frame()
	if got := tt.Cursor(); got != ui.CursorCrosshair {
		t.Fatalf("cursor off membrane = %v, want crosshair", got)
	}
}

func TestMembraneSidebarLayout(t *testing.T) {
	for _, size := range []struct {
		name string
		w, h int
	}{{"normal", 1440, 900}, {"minimum", 1080, 700}} {
		t.Run(size.name, func(t *testing.T) {
			a := newApp()
			tt := ui.NewTester(a.view, size.w, size.h)
			tt.Frame()
			if a.theme.Dark {
				t.Fatal("the light demo inherited a dark widget theme")
			}
			for _, name := range []string{"Surface depth", "Surface motion", "Reset camera"} {
				info, ok := tt.Access(name)
				if !ok || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
					t.Fatalf("%s is not laid out: %+v, %v", name, info, ok)
				}
				if info.Bounds.X < float32(size.w-sidebarWidth) || info.Bounds.X+info.Bounds.W > float32(size.w) {
					t.Fatalf("%s is outside the sidebar: %+v", name, info.Bounds)
				}
				if info.Bounds.Y < 0 || info.Bounds.Y+info.Bounds.H > float32(size.h) {
					t.Fatalf("%s is outside the window: %+v", name, info.Bounds)
				}
			}
			motion, _ := tt.Access("Surface motion")
			reset, _ := tt.Access("Reset camera")
			if motion.Bounds.Y+motion.Bounds.H >= reset.Bounds.Y {
				t.Fatalf("sidebar controls overlap: motion %+v, reset %+v", motion.Bounds, reset.Bounds)
			}
			was := a.motion
			if err := tt.Click("Surface motion"); err != nil {
				t.Fatal(err)
			}
			if a.motion == was {
				t.Fatal("Surface motion did not toggle")
			}
		})
	}
}

func screenPointForLayerBox(a *app, box ui.Rect) (float32, float32, bool) {
	if a.viewBox.W <= 0 || a.viewBox.H <= 0 || box.W <= 0 || box.H <= 0 {
		return 0, 0, false
	}
	return screenPointForLayer(a, box.X+box.W/2, box.Y+box.H/2)
}

func layerBox(t *testing.T, a *app, name string) ui.Rect {
	t.Helper()
	info, ok := a.layer.Access(name)
	if !ok || info.Bounds.W <= 0 || info.Bounds.H <= 0 {
		t.Fatalf("layer has no laid-out accessible target %q", name)
	}
	return info.Bounds
}

func screenPointForLayer(a *app, x, y float32) (float32, float32, bool) {
	uv := ui.UV{
		U: x / layerWidth,
		V: y / layerHeight,
	}
	p, ok := a.surf.SurfaceToScreen(uv)
	if !ok {
		return 0, 0, false
	}
	return a.viewBox.X + p.X, a.viewBox.Y + p.Y, true
}
