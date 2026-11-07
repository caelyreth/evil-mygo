//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package e2e

import (
	"os"
	"testing"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo"
	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/internal/wait"
	"github.com/caelyreth/evil-mygo/ui"
)

// A static canvas must draw without clicks or focus changes, including
// after a hidden window's initial Metal acquisition was occluded.
func TestContentWebGPUCanvasDrawsWithoutInput(t *testing.T) {
	if _, ok := webgpu.RuntimePath(); !ok || os.Getenv("MYGO_GPU") == "0" {
		t.Skip("requires the native WebGPU renderer")
	}
	for _, hidden := range []bool{false, true} {
		name := "visible"
		if hidden {
			name = "hidden"
		}
		t.Run(name, func(t *testing.T) {
			ready, drawn := make(chan struct{}, 1), make(chan struct{}, 1)
			mygo.RunOnMain(func() {
				canvas := ui.NewWebGPUCanvas(64, 64, func(f ui.WebGPUFrame) error {
					view, err := f.Texture.CreateView(nil)
					if err != nil {
						return err
					}
					defer view.Release()
					pass, err := f.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
						ColorAttachments: []wgpu.RenderPassColorAttachment{{
							View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
							ClearValue: wgpu.Color{R: 0.2, G: 0.6, B: 0.4, A: 1},
						}},
					})
					if err != nil {
						return err
					}
					pass.End()
					pass.Release()
					select {
					case drawn <- struct{}{}:
					default:
					}
					return nil
				})
				w := newWindow(t, mygo.WindowOptions{
					Title: "Static GPU first frame", Width: 320, Height: 240,
					Hidden: hidden, AlwaysOnTop: true,
					Content: ui.View(func(c *ui.Context) { ui.Image(c, canvas.Bitmap()).Fill() }),
				})
				w.OnReadyToShow(func() {
					if hidden {
						w.Show()
					}
					ready <- struct{}{}
				})
			})
			wait.For(t, "the static GPU window ready to show", ready)
			wait.For(t, "the static GPU canvas drawn without input", drawn)
		})
	}
}
