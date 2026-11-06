// Layer-surface renders an independent native UILayer into a Metal target.
// The left side is the GPU output; the right side is the host window's native
// controls. Clicks on the left are mapped through the layer's planar surface.
//
//	go run ./examples/layer-surface
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	layerWidth  = 900
	layerHeight = 700
	panelWidth  = 320
)

type app struct {
	layer   *ui.UILayer
	canvas  *ui.MetalCanvas
	surface *ui.PlanarSurface
	clicks  int
	name    string
}

func newApp() *app {
	a := &app{name: "A native layer"}
	a.surface = ui.NewPlanarSurface(ui.Rect{W: layerWidth, H: layerHeight})
	a.layer = ui.NewUILayer(a.layerView, layerWidth, layerHeight)
	a.canvas = ui.NewMetalCanvas(layerWidth, layerHeight, a.draw)
	a.canvas.SetSource(a.layer.Bitmap())
	return a
}

func (a *app) draw(frame ui.MetalFrame) error {
	return presentLayer(frame)
}

func (a *app) view(c *ui.Context) {
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	ui.Row(c).Fill().Background(ui.Hex("#eeeeee")).Children(func() {
		target := ui.Box(c).Width(layerWidth).Shrink(0).FillHeight().Clip()
		target.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputPointerDown, ui.InputPointerMove, ui.InputPointerUp, ui.InputScroll:
				a.layer.HandleSurfaceInput(a.surface, ev)
				a.layer.Render()
				a.canvas.Invalidate()
				return true
			}
			return false
		})
		target.Children(func() {
			ui.Image(c, a.canvas.Bitmap()).Fill()
		})

		ui.Column(c).Grow(1).FillHeight().Padding(28).Gap(18).
			Background(ui.Hex("#161616")).Children(func() {
			ui.Text(c, "MYGO / LAYER SURFACE").Font("monospace").FontSize(10).TextColor(ui.Hex("#b8b8b8"))
			ui.Text(c, "A layer,\nnot a screenshot.").Font("Baskerville").FontSize(38).LineHeight(.95).TextColor(ui.Hex("#f2f2f2"))
			ui.Text(c, "The left side is an independent native UILayer sampled by a Metal texture.").FontSize(12).LineHeight(1.5).TextColor(ui.Hex("#999999"))
			ui.Box(c).Height(1).Background(ui.Hex("#343434"))
			ui.Column(c).Padding(16).Radius(12).Background(ui.Hex("#222222")).Gap(4).Children(func() {
				ui.Text(c, "ACTIONS").Font("monospace").FontSize(10).TextColor(ui.Hex("#999999"))
				ui.Textf(c, "%02d", a.clicks).Font("Baskerville").FontSize(52).TextColor(ui.Hex("#f4f4f4"))
			})
			ui.Text(c, "Click the native controls rendered inside the Metal surface. The host maps pointer coordinates to the layer's DIP space before routing input.").FontSize(11).LineHeight(1.5).TextColor(ui.Hex("#999999"))
			ui.Spacer(c)
			ui.Text(c, "ui.NewUILayer\nMetalCanvas.SetSource\nPlanarSurface").Font("monospace").FontSize(10).LineHeight(1.6).TextColor(ui.Hex("#b8b8b8"))
		})
	})
}

func (a *app) layerView(c *ui.Context) {
	ui.Column(c).Fill().Padding(52).Gap(20).Background(ui.Hex("#ededed")).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "NATIVE LAYER / 01").Font("monospace").FontSize(12).TextColor(ui.Hex("#555555"))
			ui.Spacer(c)
			ui.Box(c).Size(10, 10).Radius(5).Background(ui.Hex("#222222"))
		})
		ui.Text(c, "A live\nnative UI.").Font("Baskerville").FontSize(78).LineHeight(.88).TextColor(ui.Hex("#181818"))
		ui.Text(c, "This form is laid out, painted and edited by MyGo off-window.").FontSize(16).LineHeight(1.45).TextColor(ui.Hex("#555555"))
		ui.TextInput(c.Key("name"), &a.name).Label("Layer name").Width(360).Height(36)
		ui.Spacer(c)
		button := ui.ButtonBase(c.Key("press-layer")).Width(300).Height(58).
			Radius(12).Background(ui.Hex("#1b1b1b")).Label("Press layer")
		button.Children(func() {
			ui.Text(c, "PRESS LAYER").Font("monospace").FontSize(14).Bold().TextColor(ui.Hex("#ffffff"))
		})
		if button.Clicked() {
			a.clicks++
		}
		ui.Textf(c, "ACTIONS  /  %02d", a.clicks).Font("monospace").FontSize(11).TextColor(ui.Hex("#555555"))
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:          "MyGo Layer Surface",
			Width:          1240,
			Height:         760,
			MinWidth:       1000,
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
