package ui

import (
	"unsafe"
	"weak"

	"github.com/egoist/mygo/internal/scene"
)

// MetalFrame is the Metal command buffer and render target of a
// MetalCanvas, as pointers to the application's native Metal objects.
// Encode into CommandBuffer while Draw runs. MyGo commands the frame
// after it; do not commit or wait on it. Device is the renderer's
// MTLDevice, so resources must belong to that device.
type MetalFrame struct {
	Device, CommandBuffer, Texture, Source   unsafe.Pointer
	Width, Height, SourceWidth, SourceHeight int
	Scale                                    float32
}

// MetalCanvas is a native Metal render target shown by a MyGo bitmap.
// Draw renders it through the window's Metal renderer, on the same device
// and command buffer, without a CPU upload. A renderer that does not draw
// Metal shows nothing.
//
// Draw runs when the canvas is first shown and after Invalidate. To
// animate, call Invalidate after changing the inputs and request a frame
// from the view.
type MetalCanvas struct {
	img *scene.Image
	bm  *Bitmap
}

// NewMetalCanvas returns a transparent w×h pixel canvas that draw renders
// into. draw may be nil, for a canvas that stays transparent.
func NewMetalCanvas(w, h int, draw func(MetalFrame) error) *MetalCanvas {
	w, h = max(w, 1), max(h, 1)
	img := scene.NewGPUCanvas(w, h, func(f scene.GPUFrame) error {
		if draw == nil {
			return nil
		}
		return draw(MetalFrame{
			Device: f.Device, CommandBuffer: f.CommandBuffer, Texture: f.Texture, Source: f.Source,
			Width: f.Width, Height: f.Height, SourceWidth: f.SourceWidth, SourceHeight: f.SourceHeight, Scale: f.Scale,
		})
	})
	return &MetalCanvas{img: img, bm: &Bitmap{img: img, w: w, h: h}}
}

// Bitmap returns the canvas's bitmap, for Image, Painter.Image and
// Painter.ImageRegion.
func (c *MetalCanvas) Bitmap() *Bitmap { return c.bm }

// Size returns the canvas's size in pixels.
func (c *MetalCanvas) Size() (w, h int) { return c.img.W, c.img.H }

// Resize changes the render target's pixel size. The texture is replaced
// on the next frame and draw runs again at the new size. Like NewMetalCanvas,
// w and h are pixels, not DIPs.
func (c *MetalCanvas) Resize(w, h int) {
	w, h = max(w, 1), max(h, 1)
	if c.img.W == w && c.img.H == h {
		return
	}
	c.img.Resize(w, h)
	c.bm.w, c.bm.h = w, h
}

// Invalidate marks the canvas to be drawn again. Call it on the main
// thread when what draw encodes changes.
func (c *MetalCanvas) Invalidate() { c.img.Changed() }

// Effect returns an Element.Effect that captures the element, sets its
// bitmap as the canvas's Source, and paints the Metal result in its place.
// The element remains a live native subtree; the Metal callback receives
// its rendered pixels rather than a DOM-like event tree.
func (c *MetalCanvas) Effect() Effect { return metalCanvasEffect{canvas: c} }

// SetSource gives the canvas a bitmap to read as Source in MetalFrame,
// for a native subtree captured through Element.Effect. The source is
// held by the frame while the callback runs; call Invalidate when it
// changes.
func (c *MetalCanvas) SetSource(b *Bitmap) {
	if b == nil {
		c.img.GPU.Source = weak.Pointer[scene.Image]{}
	} else {
		c.img.GPU.Source = weak.Make(b.img)
	}
	c.img.Changed()
}

type metalCanvasEffect struct{ canvas *MetalCanvas }

func (m metalCanvasEffect) EffectMargin() Point { return Point{} }

func (m metalCanvasEffect) PaintEffect(p *Painter, box Rect, content *Bitmap, _ Point) {
	m.canvas.SetSource(content)
	srcW, srcH := m.canvas.Size()
	src := Rect{W: float32(srcW), H: float32(srcH)}
	p.ImageRegion(m.canvas.Bitmap(), src, box, ImageOptions{})
}
