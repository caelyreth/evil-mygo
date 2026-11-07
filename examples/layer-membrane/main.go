//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// Layer-membrane presents an independent native UILayer on a silk flag.
// CPU hit testing and WebGPU share one triangulated sheet, so controls keep
// their place even when the cloth folds or is viewed from the side.
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

	fieldOfView = 0.60
	panelLift   = 0.28
	panelHalfH  = 0.70
	groundY     = -0.92
)

type camera struct {
	x, y, z float32
	yaw     float32
	pitch   float32
}

type app struct {
	canvas     *ui.WebGPUCanvas
	renderer   *membraneRenderer
	layer      *ui.UILayer
	surf       *membraneSurface
	theme      *ui.Theme
	sceneTheme *ui.Theme
	host       ui.Handle
	ctx        *ui.Context

	cam       camera
	waveTime  float32
	amplitude float32
	motion    bool
	wind      float64

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
	sceneTheme := ui.DarkTheme()
	sceneTheme.Background = ui.Hex("#0d131b")
	sceneTheme.Surface = ui.Hex("#151f2a")
	sceneTheme.SurfaceHover = ui.Hex("#202e3b")
	sceneTheme.SurfacePressed = ui.Hex("#293b4b")
	sceneTheme.Border = ui.Hex("#2c3a48")
	sceneTheme.Text = ui.Hex("#ece9df")
	sceneTheme.TextMuted = ui.Hex("#94a2af")
	sceneTheme.Accent = ui.Hex("#d3c7a9")
	sceneTheme.AccentHover = ui.Hex("#e4d9be")
	sceneTheme.AccentPressed = ui.Hex("#b6aa8e")
	sceneTheme.AccentText = ui.Hex("#111820")
	sceneTheme.Radius = 8
	sceneTheme.Font = "Avenir Next"
	a := &app{
		cam:        initialCamera(),
		amplitude:  1,
		wind:       1,
		motion:     true,
		message:    "A little wind. A living flag.",
		theme:      theme,
		sceneTheme: sceneTheme,
		renderer:   &membraneRenderer{},
	}
	a.surf = &membraneSurface{cam: a.cam, amplitude: a.amplitude, version: 1}
	a.layer = ui.NewUILayer(a.layerView, layerWidth, layerHeight)
	a.canvas = ui.NewWebGPUCanvas(viewportWidth, viewportHeight, a.draw)
	a.canvas.SetSource(a.layer.Bitmap())
	return a
}

func (a *app) draw(frame ui.WebGPUFrame) error {
	return a.renderer.draw(frame, a.surf)
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.sceneTheme)
	a.ctx = c
	a.layer.SetScale(c.PixelsPerDIP())
	a.tick(c)
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	ui.Row(c).Fill().Background(ui.Hex("#0d131b")).Children(func() {
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
		Label("Interactive native UI silk flag")
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
			_, forward, _, _ := a.surf.cameraBasis()
			a.cam.x += forward.X * ev.DY * 0.006
			a.cam.y += forward.Y * ev.DY * 0.006
			a.cam.z += forward.Z * ev.DY * 0.006
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
		Background(ui.Hex("#f4f0e6")).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "MYGO / SILK STUDY").Font("monospace").FontSize(12).TextColor(muted)
			ui.Spacer(c)
			ui.Text(c, "LIVE NATIVE LAYER").Font("monospace").FontSize(11).TextColor(muted)
		})
		ui.Text(c, "A little wind.\nA living flag.").Font("Baskerville").FontSize(74).LineHeight(.88).TextColor(ink)
		ui.Text(c, "One sheet of silk, carrying a native interface. These controls follow every fold: the cloth you see and the surface you touch are the same mesh.").
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
	ink, muted := ui.Hex("#ece9df"), ui.Hex("#94a2af")
	card, line := ui.Hex("#151f2a"), ui.Hex("#2c3a48")
	ui.Column(c).Width(sidebarWidth).Shrink(0).FillHeight().Padding(22, 24, 20, 24).
		Gap(18).Background(ui.Hex("#0d131b")).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "SILK / 01").Font("monospace").FontSize(10).TextColor(muted)
			ui.Spacer(c)
			ui.Row(c).Gap(6).Padding(4, 8).Radius(20).Border(1, line).Children(func() {
				ui.Box(c).Size(6, 6).Radius(3).Background(ink)
				ui.Text(c, "LIVE / WEBGPU").Font("monospace").FontSize(9).TextColor(ink)
			})
		})
		ui.Column(c).Gap(7).Children(func() {
			ui.Text(c, "Silk,\nin motion.").Font("Baskerville").FontSize(38).LineHeight(.93).TextColor(ink)
			ui.Text(c, "A single light sheet in a dark wind tunnel.").FontSize(12).LineHeight(1.4).TextColor(muted)
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
				ui.Text(c, "WIND STRENGTH").Font("monospace").FontSize(9).TextColor(muted)
				ui.Spacer(c)
				ui.Textf(c, "%.2f×", a.wind).Font("monospace").FontSize(10).TextColor(ink)
			})
			ui.Slider(c.Key("wind"), &a.wind, 0.25, 1.6).FillWidth().Label("Wind strength")
			if amplitude := float32(a.wind); amplitude != a.amplitude {
				a.amplitude = amplitude
				a.surf.setState(a.cam, a.waveTime, a.amplitude)
				a.canvas.Invalidate()
			}
		})
		ui.Row(c).FillWidth().Padding(10, 12).Radius(9).Background(card).Border(1, line).
			AlignItems(ui.Center).Children(func() {
			ui.Checkbox(c.Key("motion"), &a.motion, "Wind motion").
				FillWidth().FontSize(12).TextColor(ink)
		})
		reset := ui.ButtonBase(c.Key("reset")).FillWidth().Height(42).Radius(8).
			Background(ink).Border(0, ui.Transparent).Cursor(ui.CursorPointer).
			Label("Reset camera")
		if reset.Hovered() {
			reset.Background(ui.Hex("#d3c7a9"))
		}
		reset.Children(func() {
			ui.Text(c, "RESET CAMERA").Font("monospace").FontSize(11).Bold().
				TextColor(ui.Hex("#111820"))
		})
		if reset.Clicked() {
			a.cam = initialCamera()
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
			ui.Text(c, "Pinned on the left. Free in the wind. Native input follows the visible folds.").
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
	mesh      *clothMesh
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
		UV:       ui.UV{U: clamp(hit.u, 0, 1), V: clamp(hit.v, 0, 1)},
		Depth:    hit.t,
		Backface: !hit.front,
	}, true
}

func (s *membraneSurface) SurfaceToScreen(uv ui.UV) (ui.Point, bool) {
	if uv.U < 0 || uv.U > 1 || uv.V < 0 || uv.V > 1 || s.w <= 0 || s.h <= 0 {
		return ui.Point{}, false
	}
	p, n := s.geometry().point(uv)
	cam, fwd, right, up := s.cameraBasis()
	rel := sub3(p, cam)
	depth := dot3(rel, fwd)
	if depth < nearPlane || depth >= farPlane {
		return ui.Point{}, false
	}
	if dot3(rel, n) >= 0 {
		return ui.Point{}, false
	}
	// A projected caret must not appear through another fold or the floor.
	direction := normalize3(rel)
	hit, ok := s.intersect(cam, direction)
	if !ok || abs32(hit.t-dot3(rel, direction)) > 0.001 || abs32(hit.u-uv.U) > 0.0003 || abs32(hit.v-uv.V) > 0.0003 {
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
	if abs32(rd.Y) < 1e-6 {
		return 0, false
	}
	t := (groundY - cam.Y) / rd.Y
	return t, t > 0
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

func initialCamera() camera {
	return camera{x: 0.38, y: 0.52, z: 2.65, yaw: -0.14, pitch: -0.09}
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:          "MyGo Silk Study",
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
