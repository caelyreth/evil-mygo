// Layer-lab bends a live native component with layerSample: refractive
// glass, fluted glass, ripples, chromatic prisms and a mosaic lens.
//
//	go run ./examples/layer-lab
//
// No webview, glass plugin or external images are involved. The component
// remains interactive, but shader distortion does not remap hit testing.
package main

import (
	"log"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Optics Lab - MyGo layerSample",
			Width:     1240,
			Height:    820,
			MinWidth:  1000,
			MinHeight: 740,
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
