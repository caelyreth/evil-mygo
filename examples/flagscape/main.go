// Flagscape is a native Metal scene inside a MyGo window. The left side is a
// static 3D panel; the right side is a small grayscale MyGo sidebar. A native
// MyGo face is captured as a texture and mapped onto the panel.
//
//	go run ./examples/flagscape
//
// WASD moves the camera, Q/E lowers and raises it, and click-dragging the
// viewport captures the pointer for mouse look. The panel itself does not
// animate. Clicking its large ACTIVATE control is resolved through a camera
// ray, the panel UV, and the control's source-space bounds.
package main

import (
	"log"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	viewportWidth  = 2200
	viewportHeight = 1800
	panelWidth     = 320
	panelLift      = 0.28
	panelThickness = 0.025
)

type camera struct {
	x, y, z float32
	yaw     float32
	pitch   float32
}

type app struct {
	canvas *ui.MetalCanvas
	cam    camera

	last    time.Time
	drag    bool
	lastX   float32
	lastY   float32
	actions int
	viewBox ui.Rect
	hitBox  ui.Rect
}

func newApp() *app {
	a := &app{cam: camera{z: 3.8, y: 0.16}}
	a.canvas = ui.NewMetalCanvas(viewportWidth, viewportHeight, a.draw)
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return drawPanel(frame, a.cam.x, a.cam.y, a.cam.z, a.cam.yaw, a.cam.pitch)
}

func (a *app) view(c *ui.Context) {
	a.tick(c)
	ui.Row(c).Fill().Background(ui.Hex("#111111")).Children(func() {
		a.viewport(c)
		a.panel(c)
	})
}

func (a *app) tick(c *ui.Context) {
	now := c.Now()
	if a.last.IsZero() {
		a.last = now
	}
	dt := min(now.Sub(a.last).Seconds(), 0.05)
	a.last = now
	a.canvas.Invalidate()

	moving := false
	fwd := float32(math.Sin(float64(a.cam.yaw)))
	back := float32(math.Cos(float64(a.cam.yaw)))
	rightX := float32(math.Cos(float64(a.cam.yaw)))
	rightZ := float32(math.Sin(float64(a.cam.yaw)))
	const speed = float32(1.7)
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
	if moving || a.drag {
		c.AnimationFrame()
	}
}

func (a *app) viewport(c *ui.Context) {
	view := ui.Box(c).Grow(1).MinWidth(0).FillHeight().Focusable().
		Label("Interactive 3D panel viewport").
		Cursor(ui.CursorCrosshair)
	if a.drag {
		view.Cursor(ui.CursorNone)
	}
	view.Cache().Effect(a.canvas.Effect())
	a.viewBox = view.LayoutBounds()
	view.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			if a.hitAction(ev.X, ev.Y) {
				a.actions++
				a.canvas.Invalidate()
				return true
			}
			a.drag = true
			a.lastX, a.lastY = ev.X, ev.Y
			a.canvas.Invalidate()
			return true
		case ui.InputPointerMove:
			if !a.drag {
				return false
			}
			a.cam.yaw += (ev.X - a.lastX) * 0.006
			a.cam.pitch = clamp(a.cam.pitch-(ev.Y-a.lastY)*0.006, -1.05, 1.05)
			a.lastX, a.lastY = ev.X, ev.Y
			a.canvas.Invalidate()
			return true
		case ui.InputPointerUp:
			if ev.Button == 0 && a.drag {
				a.drag = false
				a.canvas.Invalidate()
				return true
			}
		case ui.InputScroll:
			a.cam.z -= ev.DY * 0.006
			a.canvas.Invalidate()
			return true
		}
		return false
	})
	view.Children(func() { a.face(c) })
}

func (a *app) face(c *ui.Context) {
	surface := ui.Box(c).Fill().IgnoreInput(true)
	surface.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{
			From: ui.Hex("#f0f0f0"), To: ui.Hex("#b8b8b8"), Angle: 135,
			Stops: []ui.GradientStop{
				{Color: ui.Hex("#f5f5f5"), At: 0},
				{Color: ui.Hex("#d6d6d6"), At: .58},
				{Color: ui.Hex("#aaaaaa"), At: 1},
			},
		}, 0)
	})
	surface.Children(func() {
		ui.Column(c).Fill().Padding(56, 48).Gap(22).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "MYGO / SURFACE").Font("monospace").FontSize(12).TextColor(ui.Hex("#3d3d3d"))
				ui.Spacer(c)
				ui.Box(c).Size(11, 11).Radius(6).Background(ui.Hex("#303030"))
			})
			ui.Column(c).Gap(9).Children(func() {
				ui.Text(c, "STATIC\nSURFACE").Font("Baskerville").FontSize(86).LineHeight(.88).TextColor(ui.Hex("#171717"))
				ui.Text(c, "A native MyGo interface, presented as one clean 3D panel.").FontSize(16).LineHeight(1.45).TextColor(ui.Hex("#4b4b4b"))
			})
			ui.Spacer(c)
			action := ui.ButtonBase(c.Key("activate")).Width(360).Height(72).AlignSelf(ui.Center).
				Radius(12).Background(ui.Hex("#1b1b1b")).Border(0, ui.Transparent).
				Label("Activate").IgnoreInput(true)
			action.Children(func() {
				ui.Text(c, "ACTIVATE").Font("monospace").FontSize(17).Bold().TextColor(ui.Hex("#ffffff"))
			})
			a.hitBox = action.LayoutBounds()
			ui.Row(c).Children(func() {
				ui.Textf(c, "ACTIVATIONS  /  %02d", a.actions).Font("monospace").FontSize(11).TextColor(ui.Hex("#3d3d3d"))
				ui.Spacer(c)
				ui.Text(c, "NATIVE UI / GRAYSCALE").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			})
		})
	})
}

func (a *app) panel(c *ui.Context) {
	ui.Column(c).Width(panelWidth).Shrink(0).FillHeight().Padding(24).Gap(16).
		Background(ui.Hex("#eeeeee")).Children(func() {
		ui.Text(c, "MYGO / SCENE").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
		ui.Text(c, "Static surface").Font("Baskerville").FontSize(32).LineHeight(.95).TextColor(ui.Hex("#171717"))
		ui.Box(c).Height(1).Background(ui.Hex("#c7c7c7"))
		ui.Column(c).Padding(16).Radius(12).Background(ui.Hex("#dddddd")).Gap(4).Children(func() {
			ui.Text(c, "ACTIVATIONS").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			ui.Textf(c, "%02d", a.actions).Font("Baskerville").FontSize(52).TextColor(ui.Hex("#171717"))
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "CAMERA").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			ui.Textf(c, "X %+.2f  Y %+.2f  Z %+.2f", a.cam.x, a.cam.y, a.cam.z).Font("monospace").FontSize(10).TextColor(ui.Hex("#313131"))
			ui.Textf(c, "YAW %+.1f°  PITCH %+.1f°", a.cam.yaw*180/math.Pi, a.cam.pitch*180/math.Pi).Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
		})
		ui.Spacer(c)
		reset := ui.ButtonBase(c.Key("reset")).FillWidth().Height(36).Radius(8).Background(ui.Hex("#1b1b1b")).Label("Reset camera")
		reset.Children(func() { ui.Text(c, "RESET CAMERA").Font("monospace").FontSize(11).TextColor(ui.Hex("#ffffff")) })
		if reset.Clicked() {
			a.cam = camera{z: 3.8, y: 0.16}
		}
		ui.Text(c, "WASD  walk     Q/E  height\nDRAG  mouse-look     WHEEL  zoom").Font("monospace").FontSize(10).LineHeight(1.5).TextColor(ui.Hex("#555555"))
	})
}

func (a *app) hitAction(x, y float32) bool {
	if a.viewBox.W <= 0 || a.viewBox.H <= 0 || a.hitBox.W <= 0 {
		return false
	}
	u, v, ok := a.panelUV(x, y, a.viewBox.W, a.viewBox.H)
	if !ok {
		return false
	}
	px := a.viewBox.X + u*a.viewBox.W
	py := a.viewBox.Y + v*a.viewBox.H
	return px >= a.hitBox.X && px < a.hitBox.X+a.hitBox.W &&
		py >= a.hitBox.Y && py < a.hitBox.Y+a.hitBox.H
}

func (a *app) panelUV(x, y, w, h float32) (float32, float32, bool) {
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	aspect := w / h
	halfH := float32(0.68)
	halfW := halfH * aspect
	ndcX := x/w*2 - 1
	ndcY := 1 - y/h*2
	fwd := v3{
		float32(math.Sin(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
		float32(math.Sin(float64(a.cam.pitch))),
		-float32(math.Cos(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
	}
	right := normalize3(cross3(fwd, v3{0, 1, 0}))
	up := normalize3(cross3(right, fwd))
	rd := normalize3(add3(fwd, add3(scale3(right, ndcX*0.74*aspect), scale3(up, ndcY*0.74))))
	if abs32(rd.z) <= 0.02 {
		return 0, 0, false
	}
	cam := v3{a.cam.x, a.cam.y, a.cam.z}
	front := a.intersectPanel(cam, rd, halfW, halfH, panelThickness)
	back := a.intersectPanel(cam, rd, halfW, halfH, -panelThickness)
	t := float32(-1)
	if front > 0 && (back <= 0 || front < back) {
		t = front
	} else if back > 0 {
		t = back
	}
	if t <= 0.01 {
		return 0, 0, false
	}
	p := add3(cam, scale3(rd, t))
	if abs32(p.x) > halfW || abs32(p.y-panelLift) > halfH {
		return 0, 0, false
	}
	u := (p.x + halfW) / (2 * halfW)
	v := (halfH - (p.y - panelLift)) / (2 * halfH)
	if rd.z > 0 {
		u = 1 - u
	}
	return u, v, true
}

func (a *app) intersectPanel(cam, rd v3, halfW, halfH, offset float32) float32 {
	if abs32(rd.z) <= 0.02 {
		return -1
	}
	t := (offset - cam.z) / rd.z
	p := add3(cam, scale3(rd, t))
	if t > 0.01 && abs32(p.x) <= halfW && abs32(p.y-panelLift) <= halfH {
		return t
	}
	return -1
}

type v3 struct{ x, y, z float32 }

func add3(a, b v3) v3           { return v3{a.x + b.x, a.y + b.y, a.z + b.z} }
func scale3(a v3, s float32) v3 { return v3{a.x * s, a.y * s, a.z * s} }
func cross3(a, b v3) v3 {
	return v3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func normalize3(a v3) v3 {
	l := float32(math.Sqrt(float64(a.x*a.x + a.y*a.y + a.z*a.z)))
	if l == 0 {
		return v3{}
	}
	return scale3(a, 1/l)
}
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
			Title:          "Flagscape",
			Width:          1440,
			Height:         900,
			MinWidth:       1080,
			MinHeight:      680,
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: 48,
			Content:        ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
