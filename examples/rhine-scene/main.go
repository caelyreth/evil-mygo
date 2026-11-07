//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// Rhine Lab is a native material archive over MyGo's GPU canvas.
package main

import (
	"log"
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	mygo.App.WhenReady(func() {
		launcher := newArchiveLauncher()
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: "RHINE LAB / Material Archive", Width: 1440, Height: 900, MinWidth: 720, MinHeight: 480,
			TitleBarStyle:        mygo.TitleBarHiddenInset,
			TitleBarHeight:       48,
			TrafficLightPosition: &mygo.Point{X: 18, Y: 17},
			BackgroundColor:      "#eae5e1",
			Content:              ui.View(launcher.view),
		})
		window.OnReadyToShow(func() { log.Print("Rhine Lab startup screen ready") })
		window.OnClosed(launcher.close)
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func sceneView(canvas *ui.WebGPUCanvas, renderer *sceneRenderer) func(*ui.Context) {
	if renderer != nil && renderer.controller != nil {
		app := newArchiveApp(canvas, renderer)
		return app.view
	}
	return func(c *ui.Context) {
		hidden := c.WindowHidden()
		ui.Box(c).Fill().Background(ui.Hex("#eae5e1")).Draw(func(p *ui.Painter, r ui.Rect) {
			if r.W <= 0 || r.H <= 0 {
				return
			}
			// Paint bounds are current; LayoutBounds is one frame behind.
			scale := p.Scale()
			canvas.Resize(int(math.Ceil(float64(r.W*scale))), int(math.Ceil(float64(r.H*scale))))
			if renderer != nil {
				if renderer.clock.advance(p.Now(), hidden, renderer.draws > 0) {
					canvas.Invalidate()
				}
				if !hidden && renderer.clock.playing() {
					p.AnimationFrameRate(60)
				}
			}
			p.Image(canvas.Bitmap(), r, ui.FillBox)
		})
	}
}
