//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// Layer-membrane presents an independent native UILayer on a moving
// heightfield. CPU hit testing and the WebGPU renderer use the same camera
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
	sidebarWidth   = 340

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
	canvas   *ui.WebGPUCanvas
	renderer *membraneRenderer
	layer    *ui.UILayer
	surf     *membraneSurface
	theme    *ui.Theme
	host     ui.Handle
	ctx      *ui.Context

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
	theme := ui.LightTheme()
	theme.Background = ui.Hex("#f8f7f3")
	theme.Surface = ui.Hex("#eceae4")
	theme.SurfaceHover = ui.Hex("#dfddd6")
	theme.SurfacePressed = ui.Hex("#d2d0c8")
	theme.Border = ui.Hex("#b8b6ae")
	theme.Text = ui.Hex("#191917")
	theme.TextMuted = ui.Hex("#4f4f49")
	theme.Accent = ui.Hex("#222220")
	theme.AccentHover = ui.Hex("#3b3b37")
	theme.AccentPressed = ui.Hex("#111110")
	theme.AccentText = ui.Hex("#f7f7f3")
	theme.InactiveAccent = ui.Hex("#8b8a83")
	theme.Selection = ui.RGBA(34, 34, 32, 0.18)
	theme.Focus = ui.RGBA(34, 34, 32, 0.42)
	theme.Scrollbar = ui.RGBA(30, 30, 28, 0.28)
	theme.Radius = 8
	theme.Font = "Avenir Next"
	a := &app{
		cam:       camera{z: 3.65, y: 0.14},
		amplitude: 1,
		depth:     1,
		motion:    true,
		message:   "A native field on a living surface.",
		theme:     theme,
		renderer:  &membraneRenderer{},
	}
	a.surf = &membraneSurface{cam: a.cam, amplitude: a.amplitude, version: 1}
	a.layer = ui.NewUILayer(a.layerView, layerWidth, layerHeight)
	a.canvas = ui.NewWebGPUCanvas(viewportWidth, viewportHeight, a.draw)
	a.canvas.SetSource(a.layer.Bitmap())
	return a
}

func (a *app) draw(frame ui.WebGPUFrame) error {
	return a.renderer.draw(
		frame,
		a.cam.x, a.cam.y, a.cam.z, a.cam.yaw, a.cam.pitch,
		a.waveTime, a.amplitude, a.surf.aspect(),
	)
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	a.ctx = c
	a.layer.SetScale(c.PixelsPerDIP())
	a.tick(c)
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	ui.Row(c).Fill().Background(ui.Hex("#e1e1dc")).Children(func() {
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
	if box.W > 0 && box.H > 0 {
		scale := c.PixelsPerDIP()
		a.canvas.Resize(
			int(math.Ceil(float64(box.W*scale))),
			int(math.Ceil(float64(box.H*scale))),
		)
	}
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
	return a.host.HandleLayerInput(a.ctx, ev)
}

func (a *app) layerView(c *ui.Context) {
	c.SetTheme(a.theme)
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
		ui.Text(c, "This text, field and button are native MyGo controls. The cloth is a separate WebGPU surface, and its input map follows the same moving geometry.").
			FontSize(16).LineHeight(1.45).TextColor(muted).Width(690)
		ui.Spacer(c)
		ui.Column(c).Width(430).Shrink(0).Gap(14).Children(func() {
			ui.TextInput(c.Key("message"), &a.message).
				Label("Message").FontSize(15).Height(50).
				Background(ui.Hex("#f2f1ec")).Border(1, ui.Hex("#b9b8b2"))
			action := ui.ButtonBase(c.Key("send-signal")).Width(430).Height(68).
				Padding(0, 22).Gap(16).Justify(ui.Start).AlignItems(ui.Center).
				Radius(10).Background(ui.Hex("#1e1e1c")).Border(0, ui.Transparent).
				Cursor(ui.CursorPointer).Label("Send signal")
			if action.Hovered() {
				action.Background(ui.Hex("#3b3b37"))
			}
			action.Children(func() {
				ui.Text(c, "SEND SIGNAL").Font("monospace").FontSize(15).Bold().
					TextColor(ui.Hex("#f7f7f3"))
				ui.Spacer(c)
				ui.Row(c).Padding(4, 8).Radius(5).
					Background(ui.Hex("#2b2b28")).Border(1, ui.Hex("#4b4b47")).
					AlignItems(ui.Center).Children(func() {
					ui.Text(c, "ENTER").Font("monospace").FontSize(10).
						TextColor(ui.Hex("#d5d5cf"))
				})
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
	ink, muted := ui.Hex("#151514"), ui.Hex("#50504a")
	card, line := ui.Hex("#ecebe5"), ui.Hex("#c0beb6")
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(22, 24, 20, 24).
		Gap(18).Background(ui.Hex("#f8f7f3")).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "MEMBRANE / 04").Font("monospace").FontSize(10).TextColor(muted)
			ui.Spacer(c)
			ui.Row(c).Gap(6).Padding(4, 8).Radius(20).Border(1, line).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(ink)
				ui.Text(c, "LIVE / WEBGPU").Font("monospace").FontSize(9).TextColor(ink)
			})
		})
		ui.Column(c).Gap(7).Children(func() {
			ui.Text(c, "A moving\nnative face.").Font("Baskerville").FontSize(34).LineHeight(.93).TextColor(ink)
			ui.Text(c, "Native controls on a live WebGPU surface.").FontSize(12).LineHeight(1.4).TextColor(muted)
		})
		ui.Row(c).FillWidth().Padding(15).Radius(10).Background(card).Border(1, line).
			AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "SIGNALS").Font("monospace").FontSize(9).TextColor(muted)
				ui.Textf(c, "%02d", a.actions).Font("Baskerville").FontSize(38).LineHeight(.95).TextColor(ink)
			})
			ui.Spacer(c)
			ui.Column(c).AlignItems(ui.End).Gap(3).Children(func() {
				ui.Text(c, "SURFACE").Font("monospace").FontSize(9).TextColor(muted)
				state := "PAUSED"
				if a.motion {
					state = "IN MOTION"
				}
				ui.Text(c, state).Font("monospace").FontSize(10).TextColor(ink)
			})
		})
		ui.Column(c).Gap(8).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "SURFACE DEPTH").Font("monospace").FontSize(9).TextColor(muted)
				ui.Spacer(c)
				ui.Textf(c, "%.2f×", a.depth).Font("monospace").FontSize(10).TextColor(ink)
			})
			ui.Slider(c.Key("depth"), &a.depth, 0.25, 1.6).FillWidth().Label("Surface depth")
			if amplitude := float32(a.depth); amplitude != a.amplitude {
				a.amplitude = amplitude
				a.surf.setState(a.cam, a.waveTime, a.amplitude)
				a.canvas.Invalidate()
			}
		})
		ui.Row(c).FillWidth().Padding(10, 12).Radius(9).Background(card).Border(1, line).
			AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c.Key("motion"), &a.motion, "Surface motion").
				FillWidth().FontSize(12).TextColor(ink)
		})
		reset := ui.ButtonBase(c.Key("reset")).FillWidth().Height(42).Radius(8).
			Background(ink).Border(0, ui.Transparent).Cursor(ui.CursorPointer).
			Label("Reset camera")
		if reset.Hovered() {
			reset.Background(ui.Hex("#383834"))
		}
		reset.Children(func() {
			ui.Text(c, "RESET CAMERA").Font("monospace").FontSize(11).Bold().
				TextColor(ui.Hex("#f7f7f3"))
		})
		if reset.Clicked() {
			a.cam = camera{z: 3.65, y: 0.14}
			a.surf.setState(a.cam, a.waveTime, a.amplitude)
			a.canvas.Invalidate()
		}
		ui.Box(c).Height(1).Background(line)
		ui.Spacer(c)
		ui.Column(c).FillWidth().Padding(14).Radius(10).Background(card).Border(1, line).
			Gap(8).Children(func() {
			ui.Text(c, "CONTROLS").Font("monospace").FontSize(9).TextColor(muted)
			ui.Text(c, "WASD    WALK        Q/E    HEIGHT\nDRAG    LOOK        WHEEL  ZOOM").
				Font("monospace").FontSize(9).LineHeight(1.75).TextColor(ink)
			ui.Text(c, "The native field keeps its caret and cursor inside the moving surface.").
				FontSize(10.5).LineHeight(1.45).TextColor(muted)
		})
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
