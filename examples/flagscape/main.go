// Flagscape is a native Metal scene inside a MyGo window. The left side is a
// full-bleed 3D viewport; the right side is ordinary MyGo UI. A native MyGo
// face is captured as a texture and mapped onto a lit, waving flag.
//
//	go run ./examples/flagscape
//
// WASD moves the camera, Q/E lowers and raises it, and click-dragging the
// viewport captures the pointer for mouse look. The right panel controls the
// cloth and camera. The Metal encoder is isolated in the cgo example.
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
	panelWidth     = 340
)

type camera struct {
	x, y, z float32
	yaw     float32
	pitch   float32
}

type app struct {
	canvas *ui.MetalCanvas
	cam    camera

	started time.Time
	last    time.Time
	time    float32
	wind    float64
	light   float64
	animate bool
	drag    bool
	lastX   float32
	lastY   float32
}

func newApp() *app {
	a := &app{
		cam:     camera{z: 3.8, y: 0.04},
		started: time.Now(),
		wind:    0.68,
		light:   0.72,
		animate: true,
	}
	a.canvas = ui.NewMetalCanvas(viewportWidth, viewportHeight, a.draw)
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return drawFlag(frame, a.time, a.cam.x, a.cam.y, a.cam.z, a.cam.yaw, a.cam.pitch, float32(a.wind), float32(a.light))
}

func (a *app) view(c *ui.Context) {
	a.tick(c)
	t := c.Theme()
	ui.Row(c).Fill().Background(t.Background).Children(func() {
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
	a.cam.x = clamp(a.cam.x, -2.8, 2.8)
	a.cam.y = clamp(a.cam.y, -0.55, 1.25)
	a.cam.z = clamp(a.cam.z, 2.0, 5.2)

	if a.animate && !c.Preferences().ReduceMotion {
		a.time += float32(dt)
		moving = true
	}
	if moving || a.drag {
		c.AnimationFrame()
	}
}

func (a *app) viewport(c *ui.Context) {
	view := ui.Box(c).Grow(1).MinWidth(0).FillHeight().Focusable().
		Label("Interactive 3D flag viewport").
		Cursor(ui.CursorCrosshair)
	if a.drag {
		view.Cursor(ui.CursorNone)
	}
	view.Cache().Effect(a.canvas.Effect())
	view.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
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
			a.cam.z = clamp(a.cam.z-ev.DY*0.006, 2.0, 5.2)
			a.canvas.Invalidate()
			return true
		}
		return false
	})
	view.Children(func() { a.face(c) })
}

func (a *app) face(c *ui.Context) {
	ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{
			From: ui.Hex("#0b3138"), To: ui.Hex("#17646a"), Angle: 135,
			Stops: []ui.GradientStop{
				{Color: ui.Hex("#092126"), At: 0},
				{Color: ui.Hex("#17646a"), At: .52},
				{Color: ui.Hex("#d47c52"), At: 1},
			},
		}, 0)
		// Keep the face free of decorative slice lines: the cloth shader
		// supplies the only surface detail.
		p.Stroke(ui.Rect{X: r.X + 18, Y: r.Y + 18, W: r.W - 36, H: r.H - 36}, ui.RGBA(244, 215, 168, .25), 0, 1)
	}).Children(func() {
		ui.Column(c).Fill().Padding(46, 48).Justify(ui.SpaceBetween).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "MYGO / FIELD UNIT 03").Font("monospace").FontSize(12).TextColor(ui.Hex("#f4d7a8"))
				ui.Spacer(c)
				ui.Box(c).Size(11, 11).Radius(6).Background(ui.Hex("#f0a36c"))
			})
			ui.Column(c).Gap(8).Children(func() {
				ui.Text(c, "A FLAG\nFOR A\nLIVE UI.").Font("Baskerville").FontSize(92).LineHeight(.86).TextColor(ui.Hex("#fff1d5"))
				ui.Text(c, "A native MyGo surface carried into a 3D world.\nThe cloth is lit; the interface is real.").FontSize(15).LineHeight(1.45).TextColor(ui.Hex("#d9e5d4"))
			})
			ui.Row(c).AlignItems(ui.End).Children(func() {
				ui.Column(c).Gap(6).Children(func() {
					ui.Text(c, "SIGNAL  /  NOMINAL").Font("monospace").FontSize(10).TextColor(ui.Hex("#f0a36c"))
					ui.Text(c, "CLOTH RIG  /  ONE FACE").Font("monospace").FontSize(10).TextColor(ui.Hex("#c5d9ce"))
				})
				ui.Spacer(c)
				ui.Text(c, "03").Font("Baskerville").FontSize(76).TextColor(ui.RGBA(255, 244, 218, .28))
			})
		})
	})
}

func (a *app) panel(c *ui.Context) {
	ui.Column(c).Width(panelWidth).Shrink(0).FillHeight().Padding(26, 24, 22, 24).
		Background(ui.Hex("#101b1e")).Gap(17).Children(func() {
		ui.Text(c, "FLAGSCAPE  /  03").Font("monospace").FontSize(11).TextColor(ui.Hex("#d67b53"))
		ui.Text(c, "The scene is\nan input.").Font("Baskerville").FontSize(40).LineHeight(.95).TextColor(ui.Hex("#ecf0df"))
		ui.Text(c, "The left viewport is a Metal scene. Its flag is textured with the captured native UI face; WASD moves the camera, and dragging captures the pointer for mouse look.").FontSize(12).LineHeight(1.55).TextColor(ui.Hex("#91a9a0"))

		ui.Box(c).Height(1).Background(ui.Hex("#294044"))
		ui.Column(c).Gap(8).Children(func() {
			ui.Text(c, "CAMERA").Font("monospace").FontSize(10).TextColor(ui.Hex("#6e9186"))
			ui.Textf(c, "X %+.2f   Y %+.2f   Z %+.2f", a.cam.x, a.cam.y, a.cam.z).Font("monospace").FontSize(11).TextColor(ui.Hex("#ecf0df"))
			ui.Textf(c, "YAW %+.1f°   PITCH %+.1f°", a.cam.yaw*180/math.Pi, a.cam.pitch*180/math.Pi).Font("monospace").FontSize(10).TextColor(ui.Hex("#91a9a0"))
		})
		ui.Column(c).Gap(5).Children(func() {
			sliderLabel(c, "Cloth wind", a.wind)
			ui.Slider(c.Key("wind"), &a.wind, 0, 1)
		})
		ui.Column(c).Gap(5).Children(func() {
			sliderLabel(c, "Sunlight", a.light)
			ui.Slider(c.Key("light"), &a.light, 0, 1)
		})
		ui.Checkbox(c.Key("animate"), &a.animate, "Cloth motion").FontSize(12).TextColor(ui.Hex("#d8e4d7"))
		ui.Spacer(c)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c.Key("reset"), "Reset view").Clicked() {
				a.cam = camera{z: 3.8, y: 0.04}
				a.wind, a.light = .68, .72
			}
			ui.Text(c, "DRAG TO LOOK").Font("monospace").FontSize(10).TextColor(ui.Hex("#6e9186"))
		})
		ui.Text(c, "WASD  walk     Q/E  height\nDRAG  mouse-look     WHEEL  zoom").Font("monospace").FontSize(10).LineHeight(1.5).TextColor(ui.Hex("#6e9186"))
		ui.Spacer(c)
		ui.Text(c, "Native MyGo controls. No WebView. No CPU pixel upload in the Metal frame.").FontSize(10).LineHeight(1.4).TextColor(ui.Hex("#6e9186"))
		ui.Text(c, "Cloth light, camera, and UI controls are independent.").FontSize(10).LineHeight(1.4).TextColor(ui.Hex("#6e9186"))
	})
}

func sliderLabel(c *ui.Context, label string, v float64) {
	ui.Row(c).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(ui.Hex("#91a9a0"))
		ui.Spacer(c)
		ui.Textf(c, "%d%%", int(v*100)).Font("monospace").FontSize(10).TextColor(ui.Hex("#d8e4d7"))
	})
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
