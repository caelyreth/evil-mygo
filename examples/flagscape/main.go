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
			From: ui.Hex("#101010"), To: ui.Hex("#4b4b4b"), Angle: 135,
			Stops: []ui.GradientStop{
				{Color: ui.Hex("#0b0b0b"), At: 0},
				{Color: ui.Hex("#3f3f3f"), At: .58},
				{Color: ui.Hex("#777777"), At: 1},
			},
		}, 0)
	})
	surface.Children(func() {
		ui.Column(c).Fill().Padding(48).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "MYGO / STATIC PANEL").Font("monospace").FontSize(12).TextColor(ui.Hex("#e6e6e6"))
				ui.Spacer(c)
				ui.Box(c).Size(11, 11).Radius(6).Background(ui.Hex("#d0d0d0"))
			})
			ui.Column(c).Gap(9).Children(func() {
				ui.Text(c, "A PANEL\nIN SPACE.").Font("Baskerville").FontSize(92).LineHeight(.86).TextColor(ui.Hex("#f4f4f4"))
				ui.Text(c, "A native MyGo surface, rendered into a static 3D panel.").FontSize(16).LineHeight(1.45).TextColor(ui.Hex("#cfcfcf"))
			})
			ui.Row(c).AlignItems(ui.Center).Gap(18).Children(func() {
				action := ui.ButtonBase(c.Key("activate")).Width(360).Height(72).
					Radius(18).Background(ui.RGBA(245, 245, 245, .16)).Border(2, ui.RGBA(245, 245, 245, .72)).
					Label("Activate panel").IgnoreInput(true)
				action.Children(func() {
					ui.Textf(c, "ACTIVATE  /  %02d", a.actions).Font("monospace").FontSize(15).TextColor(ui.Hex("#f7f7f7"))
					ui.Spacer(c)
					ui.Text(c, "CLICK").Font("monospace").FontSize(11).TextColor(ui.Hex("#bdbdbd"))
				})
				a.hitBox = action.LayoutBounds()
				ui.Column(c).Gap(4).Children(func() {
					ui.Text(c, "SIGNAL / NOMINAL").Font("monospace").FontSize(10).TextColor(ui.Hex("#d6d6d6"))
					ui.Text(c, "MODEL / STATIC PANEL").Font("monospace").FontSize(10).TextColor(ui.Hex("#a9a9a9"))
				})
				ui.Spacer(c)
				ui.Text(c, "03").Font("Baskerville").FontSize(66).TextColor(ui.RGBA(255, 255, 255, .24))
			})
		})
	})
}

func (a *app) panel(c *ui.Context) {
	ui.Column(c).Width(panelWidth).Shrink(0).FillHeight().Padding(24).Gap(18).
		Background(ui.Hex("#151515")).Children(func() {
		ui.Text(c, "FLAGSCAPE / STATIC").Font("monospace").FontSize(10).TextColor(ui.Hex("#bcbcbc"))
		ui.Text(c, "A panel,\nnot a flag.").Font("Baskerville").FontSize(38).LineHeight(.95).TextColor(ui.Hex("#f0f0f0"))
		ui.Text(c, "The viewport is a Metal scene. The panel is a captured native MyGo face.").FontSize(12).LineHeight(1.5).TextColor(ui.Hex("#929292"))
		ui.Box(c).Height(1).Background(ui.Hex("#343434"))
		ui.Column(c).Padding(16).Radius(12).Background(ui.Hex("#202020")).Gap(6).Children(func() {
			ui.Text(c, "ACTIVATIONS").Font("monospace").FontSize(10).TextColor(ui.Hex("#929292"))
			ui.Textf(c, "%02d", a.actions).Font("Baskerville").FontSize(48).TextColor(ui.Hex("#f4f4f4"))
		})
		ui.Column(c).Gap(5).Children(func() {
			ui.Text(c, "CAMERA").Font("monospace").FontSize(10).TextColor(ui.Hex("#929292"))
			ui.Textf(c, "X %+.2f  Y %+.2f  Z %+.2f", a.cam.x, a.cam.y, a.cam.z).Font("monospace").FontSize(10).TextColor(ui.Hex("#d8d8d8"))
			ui.Textf(c, "YAW %+.1f°  PITCH %+.1f°", a.cam.yaw*180/math.Pi, a.cam.pitch*180/math.Pi).Font("monospace").FontSize(10).TextColor(ui.Hex("#929292"))
		})
		ui.Spacer(c)
		if ui.Button(c.Key("reset"), "Reset view").Clicked() {
			a.cam = camera{z: 3.8, y: 0.16}
		}
		ui.Text(c, "WASD  walk     Q/E  height\nDRAG  mouse-look     WHEEL  zoom").Font("monospace").FontSize(10).LineHeight(1.5).TextColor(ui.Hex("#929292"))
		ui.Text(c, "Click the large ACTIVATE control inside the panel. The click is ray-hittested through the 3D surface.").FontSize(10).LineHeight(1.45).TextColor(ui.Hex("#929292"))
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
