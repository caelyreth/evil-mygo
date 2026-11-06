// Metal-layer renders a live native MyGo subtree through an application
// Metal compute pass. The subtree is captured by Element.Effect; the
// compute pass reads its texture and writes a new native image. The source
// controls still receive input at their original geometry.
//
//	go run ./examples/metal-layer
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
	name    string
	volume  float64
	done    bool
}

func newApp() *app {
	a := &app{
		started: time.Now(), animate: true, energy: 0.7, name: "Mara Chen", volume: 0.62,
	}
	a.canvas = ui.NewMetalCanvas(900, 520, a.draw)
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return drawLayer(frame, float32(time.Since(a.started).Seconds()), float32(a.energy))
}

func (a *app) view(c *ui.Context) {
	if a.animate {
		a.canvas.Invalidate()
		c.AnimationFrame()
	}
	ui.Column(c).Fill().Padding(30, 36, 26, 36).Gap(18).Background(ui.Hex("#e8e4d8")).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "MYGO / METAL LAYER").Font("monospace").FontSize(10).TextColor(ui.Hex("#64736b"))
			ui.Spacer(c)
			ui.Text(c, "NATIVE UI -> GPU TEXTURE -> COMPUTE").Font("monospace").FontSize(10).TextColor(ui.Hex("#a55b3d"))
		})
		ui.Row(c).Grow(1).Gap(32).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(12).Children(func() {
				ui.Text(c, "Still a control.").Font("Baskerville").FontSize(45).TextColor(ui.Hex("#213d3c"))
				ui.Text(c, "The text field, slider and button are a real native subtree captured by Element.Effect. The Metal pass bends the captured texture.").FontSize(13).LineHeight(1.55).TextColor(ui.Hex("#697168"))
				source := ui.Box(c).Fill().MinHeight(300).Radius(18).Clip().Background(ui.Hex("#fffdf7")).Shadow(0, 10, 25, 0, ui.RGBA(34, 57, 49, 0.13))
				source.Cache().Effect(a.canvas.Effect())
				source.Children(func() { a.form(c) })
			})
			ui.Column(c).Width(260).Gap(14).Children(func() {
				ui.Text(c, "The metal pass").Font("Baskerville").FontSize(30).TextColor(ui.Hex("#213d3c"))
				ui.Text(c, "Source is the captured native layer. Target is a render texture. Both live on the same Metal device and command buffer.").FontSize(12).LineHeight(1.5).TextColor(ui.Hex("#697168"))
				ui.Column(c).Gap(5).Children(func() {
					ui.Text(c, "Wave energy").FontSize(12).TextColor(ui.Hex("#697168"))
					ui.Slider(c.Key("energy"), &a.energy, 0, 1)
				})
				ui.Checkbox(c.Key("animate"), &a.animate, "Animate waves").FontSize(12)
				ui.Text(c, "Hit testing, focus and keyboard input still use the original component geometry. The effect changes pixels, not interaction.").FontSize(11).LineHeight(1.45).TextColor(ui.Hex("#a55b3d"))
				ui.Spacer(c)
				ui.Text(c, "Element.Cache keeps the source texture stable while only the Metal pass animates.").FontSize(11).LineHeight(1.45).TextColor(ui.Hex("#8b9287"))
			})
		})
		ui.Row(c).Children(func() {
			ui.Text(c, "ui.MetalCanvas.Effect()").Font("monospace").FontSize(10).TextColor(ui.Hex("#64736b"))
			ui.Spacer(c)
			ui.Text(c, "NO WEBVIEW / NO CPU READBACK").Font("monospace").FontSize(10).TextColor(ui.Hex("#a55b3d"))
		})
	})
}

func (a *app) form(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Row(c).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, "PROFILE CARD").Font("monospace").FontSize(10).TextColor(ui.Hex("#a55b3d"))
				ui.Text(c, "A small native specimen").Font("Baskerville").FontSize(25).TextColor(ui.Hex("#213d3c"))
			})
			ui.Spacer(c)
			ui.Box(c).Size(46, 46).Radius(23).Background(ui.Hex("#d7e2d5")).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Fill(ui.Rect{X: r.X + r.W*.25, Y: r.Y + r.H*.25, W: r.W * .5, H: r.H * .5}, ui.Hex("#507c70"), r.W)
			})
		})
		ui.TextInput(c.Key("name"), &a.name).Label("Display name").FontSize(13).Background(ui.Hex("#f2f0e7")).Border(1, ui.Hex("#e0ded2"))
		ui.Column(c).Gap(4).Children(func() {
			ui.Row(c).Children(func() {
				ui.Text(c, "Presence").FontSize(12).TextColor(t.TextMuted)
				ui.Spacer(c)
				ui.Textf(c, "%d%%", int(a.volume*100)).Font("monospace").FontSize(11).TextColor(ui.Hex("#507c70"))
			})
			ui.Slider(c.Key("volume"), &a.volume, 0, 1)
		})
		ui.Row(c).Gap(9).Children(func() {
			if ui.PrimaryButton(c.Key("done"), "Acknowledge").Label("Acknowledge profile").Clicked() {
				a.done = true
			}
			if a.done {
				ui.Text(c, "Acknowledged").FontSize(12).TextColor(ui.Hex("#507c70"))
			}
		})
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "MyGo Metal Layer", Width: 1120, Height: 720, MinWidth: 900, MinHeight: 580, Content: ui.View(a.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
