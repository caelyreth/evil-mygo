// Metal-canvas is a native MyGo window containing a custom Metal render
// target. The target is encoded directly into MyGo's frame command buffer
// and sampled without a CPU upload; the surrounding controls are ordinary
// native UI.
//
//	go run ./examples/metal-canvas
//
// The Metal encoder lives in metal_draw_darwin.m and is compiled only by
// cgo on macOS. Without cgo the canvas remains a transparent native image.
package main

import (
	"log"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type app struct {
	canvas  *ui.MetalCanvas
	started time.Time
	animate bool
	energy  float64
	tint    float64
	title   string
}

func newApp() *app {
	a := &app{
		started: time.Now(),
		animate: true,
		energy:  0.72,
		tint:    0.38,
		title:   "A Metal surface inside MyGo",
	}
	a.canvas = ui.NewMetalCanvas(900, 520, a.draw)
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return drawMetal(frame, float32(time.Since(a.started).Seconds()), float32(a.energy), float32(a.tint))
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	// A view rebuild is a request for the canvas to reflect the controls,
	// whether the animation is running or not.
	a.canvas.Invalidate()
	if a.animate {
		c.AnimationFrame()
	}
	ui.Column(c).Fill().Padding(28).Gap(18).Background(ui.Hex("#10191d")).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "MYGO / METAL CANVAS").Font("monospace").FontSize(11).TextColor(ui.Hex("#7ea69b"))
			ui.Spacer(c)
			ui.Text(c, "SAME DEVICE / SAME COMMAND BUFFER").Font("monospace").FontSize(10).TextColor(ui.Hex("#57716b"))
		})
		ui.Row(c).Grow(1).Gap(22).Children(func() {
			preview := ui.Box(c).Grow(1).MinWidth(0).Radius(16).Clip().Background(ui.Hex("#071113"))
			preview.Children(func() {
				ui.Image(c, a.canvas.Bitmap()).Fill()
				ui.Box(c).Absolute().Left(18).Bottom(16).Children(func() {
					ui.Text(c, "GPU TEXTURE / NO READBACK").Font("monospace").FontSize(10).TextColor(ui.Hex("#9abeb2"))
				})
			})
			ui.Column(c).Width(270).Gap(16).Children(func() {
				ui.Text(c, "A native window\nwith a GPU guest.").Font("Baskerville").FontSize(31).TextColor(ui.Hex("#e4eedf"))
				ui.Text(c, "The compute pass writes into MyGo's render target. MyGo samples that target in the same frame and draws the controls around it.").FontSize(12).LineHeight(1.5).TextColor(ui.Hex("#94aaa1"))
				ui.TextInput(c.Key("title"), &a.title).Label("Canvas title").Background(ui.Hex("#18282c")).Border(1, ui.Hex("#2b4145")).TextColor(ui.Hex("#e4eedf"))
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, "Energy").FontSize(12).TextColor(ui.Hex("#94aaa1"))
					ui.Slider(c.Key("energy"), &a.energy, 0, 1)
				})
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, "Tint").FontSize(12).TextColor(ui.Hex("#94aaa1"))
					ui.Slider(c.Key("tint"), &a.tint, 0, 1)
				})
				ui.Checkbox(c.Key("animate"), &a.animate, "Animate Metal").FontSize(12).TextColor(ui.Hex("#94aaa1"))
				ui.Spacer(c)
				ui.Text(c, "The surrounding field is real MyGo input. The surface is a render target, not a screenshot.").FontSize(11).LineHeight(1.45).TextColor(ui.Hex("#6f8981"))
			})
		})
		ui.Row(c).Children(func() {
			ui.Text(c, a.title).FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "ui.NewMetalCanvas  •  MTLCommandBuffer  •  ui.Image").Font("monospace").FontSize(10).TextColor(ui.Hex("#57716b"))
		})
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "MyGo Metal Canvas",
			Width:     1120,
			Height:    720,
			MinWidth:  880,
			MinHeight: 540,
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
