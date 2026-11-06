// Layer-membrane presents an independent native UILayer on a moving
// heightfield. CPU hit testing and the Metal renderer use the same camera
// and wave state, so controls keep their place on the lit surface.
//
//	go run ./examples/layer-membrane
package main

import (
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	viewportWidth  = 1800
	viewportHeight = 1200
	layerWidth     = 1200
	layerHeight    = 800
	sidebarWidth   = 320

	fieldOfView = 0.74
	panelLift   = 0.28
	panelHalfH  = 0.70
	groundY     = -0.74
)

type camera struct {
	x, y, z float32
	yaw     float32
	pitch   float32
}

type app struct {
	canvas *ui.MetalCanvas
	layer  *ui.UILayer
	surf   *membraneSurface
	host   ui.Handle
	ctx    *ui.Context

	cam       camera
	waveTime  float32
	amplitude float32
	motion    bool
	depth     float64

	last  time.Time
	drag  bool
	lastX float32
	lastY float32

	actions int
	message string
	viewBox ui.Rect
}

func newApp() *app {
	a := &app{
		cam:       camera{z: 3.65, y: 0.14},
		amplitude: 1,
		depth:     1,
		motion:    true,
		message:   "A native field on a living surface.",
	}
	a.surf = &membraneSurface{cam: a.cam, amplitude: a.amplitude, version: 1}
	a.layer = ui.NewUILayerWithScale(a.layerView, layerWidth, layerHeight, 1.5)
	a.canvas = ui.NewMetalCanvas(viewportWidth, viewportHeight, a.draw)
	a.canvas.SetSource(a.layer.Bitmap())
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return drawMembrane(
		frame,
		a.cam.x, a.cam.y, a.cam.z, a.cam.yaw, a.cam.pitch,
		a.waveTime, a.amplitude, a.surf.aspect(),
	)
}

func (a *app) view(c *ui.Context) {
	a.ctx = c
	a.tick(c)
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	ui.Row(c).Fill().Background(ui.Hex("#ddddda")).Children(func() {
		a.viewport(c)
		a.sidebar(c)
	})
}

func (a *app) tick(c *ui.Context) {
	now := c.Now()
	if a.last.IsZero() {
		a.last = now
	}
	dt := min(now.Sub(a.last).Seconds(), 0.05)
	a.last = now

	if a.motion {
		a.waveTime += float32(dt) * 1.15
		a.canvas.Invalidate()
		c.AnimationFrame()
	}

	textActive, _ := a.layer.TextInputActive()
	moving := false
	if !textActive {
		fwd := float32(math.Sin(float64(a.cam.yaw)))
		back := float32(math.Cos(float64(a.cam.yaw)))
		rightX := float32(math.Cos(float64(a.cam.yaw)))
		rightZ := float32(math.Sin(float64(a.cam.yaw)))
		const speed = float32(1.55)
		if c.KeyDown(ui.KeyW) {
			a.cam.x += fwd * speed * float32(dt)
			a.cam.z -= back * speed * float32(dt)
			moving = true
		}
		if c.KeyDown(ui.KeyS) {
			a.cam.x -= fwd * speed * float32(dt)
			a.cam.z += back * speed * float32(dt)
			moving = true
		}
		if c.KeyDown(ui.KeyA) {
			a.cam.x -= rightX * speed * float32(dt)
			a.cam.z -= rightZ * speed * float32(dt)
			moving = true
		}
		if c.KeyDown(ui.KeyD) {
			a.cam.x += rightX * speed * float32(dt)
			a.cam.z += rightZ * speed * float32(dt)
			moving = true
		}
		if c.KeyDown(ui.KeyQ) {
			a.cam.y -= speed * float32(dt)
			moving = true
		}
		if c.KeyDown(ui.KeyE) {
			a.cam.y += speed * float32(dt)
			moving = true
		}
	}
	if moving || a.drag {
		a.canvas.Invalidate()
		c.AnimationFrame()
	}
	a.surf.setState(a.cam, a.waveTime, a.amplitude)
}

func (a *app) viewport(c *ui.Context) {
	view := ui.Box(c).Grow(1).MinWidth(0).FillHeight().AutoFocus().Bind(&a.host).
		Label("Interactive native UI membrane")
	box := view.LayoutBounds()
	a.viewBox = box
	a.surf.setViewport(box.W, box.H)
	view.HostLayer(a.layer, a.surf)

	if a.drag {
		view.Cursor(ui.CursorNone)
	} else if a.layer.Cursor() == ui.CursorDefault {
		view.Cursor(ui.CursorCrosshair)
	}
	view.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button == 0 && a.routeLayer(ev) {
				return true
			}
			if ev.Button == 0 {
				a.drag = true
				a.lastX, a.lastY = ev.X, ev.Y
				a.canvas.Invalidate()
				return true
			}
		case ui.InputPointerMove:
			if a.drag {
				a.cam.yaw += (ev.X - a.lastX) * 0.006
				a.cam.pitch = clamp(a.cam.pitch-(ev.Y-a.lastY)*0.006, -1.05, 1.05)
				a.lastX, a.lastY = ev.X, ev.Y
				a.canvas.Invalidate()
				return true
			}
			return a.routeLayer(ev)
		case ui.InputPointerUp:
			if ev.Button == 0 && a.drag {
				a.drag = false
				a.canvas.Invalidate()
				return true
			}
			return a.routeLayer(ev)
		case ui.InputScroll:
			if a.routeLayer(ev) {
				return true
			}
			a.cam.z = max(1.35, a.cam.z-ev.DY*0.006)
			a.canvas.Invalidate()
			return true
		case ui.InputKeyDown, ui.InputKeyUp, ui.InputText, ui.InputCompose, ui.InputCommand,
			ui.InputPointerLeave:
			return a.routeLayer(ev)
		}
		return false
	})
	view.Children(func() {
		ui.Image(c, a.canvas.Bitmap()).Fill()
	})
}

func (a *app) routeLayer(ev ui.InputEvent) bool {
	taken := a.host.HandleLayerInput(a.ctx, ev)
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	return taken
}

func (a *app) layerView(c *ui.Context) {
	muted := ui.Hex("#575753")
	ink := ui.Hex("#171715")
	ui.Column(c).Fill().Padding(54, 50).Gap(18).
		Background(ui.Hex("#e6e5e0")).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "MYGO / MEMBRANE 04").Font("monospace").FontSize(12).TextColor(muted)
			ui.Spacer(c)
			ui.Text(c, "LIVE NATIVE LAYER").Font("monospace").FontSize(11).TextColor(muted)
		})
		ui.Text(c, "A surface\nthat answers.").Font("Baskerville").FontSize(74).LineHeight(.88).TextColor(ink)
		ui.Text(c, "This text, field and button are native MyGo controls. The cloth is a separate Metal surface, and its input map follows the same moving geometry.").
			FontSize(16).LineHeight(1.45).TextColor(muted).Width(690)
		ui.Spacer(c)
		ui.Column(c).Width(430).Shrink(0).Gap(12).Children(func() {
			ui.TextInput(c.Key("message"), &a.message).
				Label("Message").FontSize(15).Height(50).
				Background(ui.Hex("#f2f1ec")).Border(1, ui.Hex("#b9b8b2"))
			action := ui.ButtonBase(c.Key("send-signal")).Width(430).Height(68).
				Radius(10).Background(ui.Hex("#1e1e1c")).Border(0, ui.Transparent).
				Cursor(ui.CursorPointer).Label("Send signal")
			if action.Hovered() {
				action.Background(ui.Hex("#3b3b37"))
			}
			action.Children(func() {
				ui.Text(c, "SEND SIGNAL").Font("monospace").FontSize(15).Bold().
					TextColor(ui.Hex("#f7f7f3"))
				ui.Spacer(c)
				ui.Text(c, "ENTER").Font("monospace").FontSize(11).TextColor(ui.Hex("#bdbdb7"))
			})
			if action.Clicked() {
				a.actions++
			}
		})
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Textf(c, "SIGNALS  /  %02d", a.actions).Font("monospace").FontSize(11).TextColor(ink)
			ui.Spacer(c)
			ui.Text(c, "RAY  ->  UV  ->  NATIVE INPUT").Font("monospace").FontSize(10).TextColor(muted)
		})
	})
}

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(24, 26).
		Background(ui.Hex("#f0efe9")).Children(func() {
		ui.Text(c, "MEMBRANE / LIVE").Font("monospace").FontSize(10).TextColor(ui.Hex("#686862"))
		ui.Text(c, "A moving\nnative face.").Font("Baskerville").FontSize(38).LineHeight(.95).TextColor(ui.Hex("#171715"))
		ui.Box(c).Height(1).Background(ui.Hex("#c9c8c1"))
		ui.Column(c).Padding(15).Radius(10).Background(ui.Hex("#e4e3dc")).Gap(3).Children(func() {
			ui.Text(c, "SIGNALS").Font("monospace").FontSize(10).TextColor(ui.Hex("#686862"))
			ui.Textf(c, "%02d", a.actions).Font("Baskerville").FontSize(44).TextColor(ui.Hex("#171715"))
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "SURFACE DEPTH").FontSize(12).TextColor(ui.Hex("#565650"))
			ui.Slider(c.Key("depth"), &a.depth, 0.25, 1.6).Label("Surface depth")
			if amplitude := float32(a.depth); amplitude != a.amplitude {
				a.amplitude = amplitude
				a.surf.setState(a.cam, a.waveTime, a.amplitude)
				a.canvas.Invalidate()
			}
		})
		ui.Checkbox(c.Key("motion"), &a.motion, "Surface motion").FontSize(12)
		if ui.Button(c.Key("reset"), "Reset camera").FillWidth().Clicked() {
			a.cam = camera{z: 3.65, y: 0.14}
			a.surf.setState(a.cam, a.waveTime, a.amplitude)
			a.canvas.Invalidate()
		}
		ui.Spacer(c)
		ui.Text(c, "WASD   walk       Q/E   height\nDRAG   mouse-look     WHEEL   zoom").
			Font("monospace").FontSize(10).LineHeight(1.55).TextColor(ui.Hex("#686862"))
		ui.Text(c, "The native field keeps the caret and cursor shape while the geometry moves.").
			FontSize(11).LineHeight(1.4).TextColor(ui.Hex("#777770"))
	})
}

type membraneSurface struct {
	cam       camera
	waveTime  float32
	amplitude float32
	w, h      float32
	version   uint64
}

func (s *membraneSurface) setViewport(w, h float32) {
	w, h = max(w, 1), max(h, 1)
	if s.w != w || s.h != h {
		s.w, s.h = w, h
		s.version++
	}
}

func (s *membraneSurface) setState(cam camera, t, amplitude float32) {
	if s.cam != cam || s.waveTime != t || s.amplitude != amplitude {
		s.cam, s.waveTime, s.amplitude = cam, t, amplitude
		s.version++
	}
}

func (s *membraneSurface) aspect() float32 {
	if s.w <= 0 || s.h <= 0 {
		return float32(layerWidth) / layerHeight
	}
	return max(s.w/s.h, 0.01)
}

func (s *membraneSurface) Version() uint64 { return s.version }

func (s *membraneSurface) ScreenToSurface(p ui.Point) (ui.SurfaceHit, bool) {
	cam, rd, ok := s.ray(p.X, p.Y)
	if !ok {
		return ui.SurfaceHit{}, false
	}
	hit, ok := s.intersect(cam, rd)
	if !ok {
		return ui.SurfaceHit{}, false
	}
	return ui.SurfaceHit{
		UV:       ui.UV{U: hit.u, V: hit.v},
		Depth:    hit.t,
		Backface: !hit.front,
	}, true
}

func (s *membraneSurface) SurfaceToScreen(uv ui.UV) (ui.Point, bool) {
	if uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 || s.w <= 0 || s.h <= 0 {
		return ui.Point{}, false
	}
	halfW := panelHalfH * s.aspect()
	p := v3{
		uv.U*2*halfW - halfW,
		panelLift + panelHalfH - uv.V*2*panelHalfH,
		s.wave(uv.U, uv.V),
	}
	cam, fwd, right, up := s.cameraBasis()
	rel := sub3(p, cam)
	depth := dot3(rel, fwd)
	if depth <= 0.01 {
		return ui.Point{}, false
	}
	n := s.normal(uv.U, uv.V)
	if dot3(normalize3(rel), n) >= 0 {
		return ui.Point{}, false
	}
	if t, ok := s.groundDepth(cam, normalize3(rel)); ok && t < depth-0.002 {
		return ui.Point{}, false
	}
	nx := dot3(rel, right) / (depth * fieldOfView * s.aspect())
	ny := dot3(rel, up) / (depth * fieldOfView)
	x := (nx + 1) * s.w / 2
	y := (1 - ny) * s.h / 2
	if x < 0 || x >= s.w || y < 0 || y >= s.h {
		return ui.Point{}, false
	}
	return ui.Point{X: x, Y: y}, true
}

type surfaceHit struct {
	t     float32
	u, v  float32
	front bool
}

func (s *membraneSurface) intersect(cam, rd v3) (surfaceHit, bool) {
	if abs32(rd.Z) < 0.015 {
		return surfaceHit{}, false
	}
	t := -cam.Z / rd.Z
	halfW := panelHalfH * s.aspect()
	var p v3
	var u, v float32
	for range 9 {
		p = add3(cam, scale3(rd, t))
		u = (p.X + halfW) / (2 * halfW)
		v = (panelLift + panelHalfH - p.Y) / (2 * panelHalfH)
		z := s.wave(clamp(u, 0, 1), clamp(v, 0, 1))
		t = (z - cam.Z) / rd.Z
	}
	p = add3(cam, scale3(rd, t))
	u = (p.X + halfW) / (2 * halfW)
	v = (panelLift + panelHalfH - p.Y) / (2 * panelHalfH)
	if t <= 0.01 || u < 0 || u > 1 || v < 0 || v > 1 {
		return surfaceHit{}, false
	}
	if floor, ok := s.groundDepth(cam, rd); ok && floor < t {
		return surfaceHit{}, false
	}
	n := s.normal(u, v)
	return surfaceHit{
		t: t, u: u, v: v, front: dot3(rd, n) < 0,
	}, true
}

func (s *membraneSurface) wave(u, v float32) float32 {
	a := sin32(6.1*u+0.82*s.waveTime) * sin32(3.2*v-0.46*s.waveTime)
	b := sin32(12.4*u-1.27*s.waveTime) * sin32(6.3*v+0.31*s.waveTime)
	return s.amplitude * (0.105*a + 0.034*b)
}

func (s *membraneSurface) normal(u, v float32) v3 {
	const e = 0.0015
	halfW := panelHalfH * s.aspect()
	zu := (s.wave(u+e, v) - s.wave(u-e, v)) / (2 * e)
	zv := (s.wave(u, v+e) - s.wave(u, v-e)) / (2 * e)
	return normalize3(v3{
		X: -zu * 2 * panelHalfH,
		Y: 2 * halfW * zv,
		Z: 4 * halfW * panelHalfH,
	})
}

func (s *membraneSurface) ray(x, y float32) (v3, v3, bool) {
	if s.w <= 0 || s.h <= 0 || x < 0 || x >= s.w || y < 0 || y >= s.h {
		return v3{}, v3{}, false
	}
	ndcX := x/s.w*2 - 1
	ndcY := 1 - y/s.h*2
	cam, fwd, right, up := s.cameraBasis()
	rd := normalize3(add3(fwd, add3(
		scale3(right, ndcX*fieldOfView*s.aspect()),
		scale3(up, ndcY*fieldOfView),
	)))
	return cam, rd, true
}

func (s *membraneSurface) groundDepth(cam, rd v3) (float32, bool) {
	if rd.Y >= -0.001 {
		return 0, false
	}
	t := (groundY - cam.Y) / rd.Y
	return t, t > 0.01
}

func (s *membraneSurface) cameraBasis() (v3, v3, v3, v3) {
	fwd := normalize3(v3{
		X: sin32(s.cam.yaw) * cos32(s.cam.pitch),
		Y: sin32(s.cam.pitch),
		Z: -cos32(s.cam.yaw) * cos32(s.cam.pitch),
	})
	right := normalize3(cross3(fwd, v3{Y: 1}))
	up := normalize3(cross3(right, fwd))
	return v3{s.cam.x, s.cam.y, s.cam.z}, fwd, right, up
}

type v3 struct{ X, Y, Z float32 }

func add3(a, b v3) v3 { return v3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func sub3(a, b v3) v3 { return v3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func scale3(a v3, s float32) v3 {
	return v3{a.X * s, a.Y * s, a.Z * s}
}
func dot3(a, b v3) float32 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func cross3(a, b v3) v3 {
	return v3{
		X: a.Y*b.Z - a.Z*b.Y,
		Y: a.Z*b.X - a.X*b.Z,
		Z: a.X*b.Y - a.Y*b.X,
	}
}
func normalize3(a v3) v3 {
	l := float32(math.Sqrt(float64(dot3(a, a))))
	if l == 0 {
		return v3{}
	}
	return scale3(a, 1/l)
}
func sin32(v float32) float32 { return float32(math.Sin(float64(v))) }
func cos32(v float32) float32 { return float32(math.Cos(float64(v))) }
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:          "MyGo Layer Membrane",
			Width:          1440,
			Height:         900,
			MinWidth:       1080,
			MinHeight:      700,
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: 48,
			Content:        ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
