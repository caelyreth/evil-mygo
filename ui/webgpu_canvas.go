//go:build darwin && ((mygo_webgpu && nofakecgo) || (cgo && !mygo_metal))

package ui

import (
	"weak"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

// WebGPUFrame is the command encoder and render target of a WebGPUCanvas.
// Encode into Encoder while Draw runs. MyGo submits the frame after it; do
// not submit, present or wait in the callback. Device is the renderer's
// WebGPU device, so every resource the callback uses belongs to that
// device.
type WebGPUFrame struct {
	Device  *wgpu.Device
	Encoder *wgpu.CommandEncoder
	Texture *wgpu.Texture
	Source  *wgpu.TextureView

	Width, Height             int
	SourceWidth, SourceHeight int
	Scale                     float32
}

// WebGPUCanvas is a WebGPU render target shown by a MyGo bitmap. Draw
// renders it through the window's WebGPU device and command encoder,
// without a CPU upload. The target is cleared to transparent before Draw,
// then sampled by MyGo in the same submitted frame.
type WebGPUCanvas struct {
	img *scene.Image
	bm  *Bitmap
}

// NewWebGPUCanvas returns a transparent w×h pixel canvas that draw renders
// into. draw may be nil, for a canvas that stays transparent.
func NewWebGPUCanvas(w, h int, draw func(WebGPUFrame) error) *WebGPUCanvas {
	w, h = max(w, 1), max(h, 1)
	img := scene.NewGPUCanvas(w, h, func(f scene.GPUFrame) error {
		if draw == nil {
			return nil
		}
		device, _ := f.DeviceObject.(*wgpu.Device)
		encoder, _ := f.EncoderObject.(*wgpu.CommandEncoder)
		texture, _ := f.TextureObject.(*wgpu.Texture)
		source, _ := f.SourceObject.(*wgpu.TextureView)
		return draw(WebGPUFrame{
			Device: device, Encoder: encoder, Texture: texture, Source: source,
			Width: f.Width, Height: f.Height,
			SourceWidth: f.SourceWidth, SourceHeight: f.SourceHeight, Scale: f.Scale,
		})
	})
	return &WebGPUCanvas{img: img, bm: &Bitmap{img: img, w: w, h: h}}
}

// Bitmap returns the canvas's bitmap, for Image, Painter.Image and
// Painter.ImageRegion.
func (c *WebGPUCanvas) Bitmap() *Bitmap { return c.bm }

// Size returns the canvas's size in pixels.
func (c *WebGPUCanvas) Size() (w, h int) { return c.img.W, c.img.H }

// Resize changes the render target's pixel size. The texture is replaced
// on the next frame and draw runs again at the new size.
func (c *WebGPUCanvas) Resize(w, h int) {
	w, h = max(w, 1), max(h, 1)
	if c.img.W == w && c.img.H == h {
		return
	}
	c.img.Resize(w, h)
	c.bm.w, c.bm.h = w, h
}

// Invalidate marks the canvas to be drawn again. Call it on the main
// thread when what draw encodes changes.
func (c *WebGPUCanvas) Invalidate() { c.img.Changed() }

// Effect captures the element and its children, publishes the captured
// bitmap as WebGPUFrame.Source and paints the canvas in place of the
// element. Interaction remains on the original native geometry.
func (c *WebGPUCanvas) Effect() Effect { return webGPUCanvasEffect{canvas: c} }

// SetSource gives the canvas a bitmap to read as Source in WebGPUFrame.
// The source is held by the frame while the callback runs; call Invalidate
// when it changes.
func (c *WebGPUCanvas) SetSource(b *Bitmap) {
	if b == nil {
		c.img.GPU.Source = weak.Pointer[scene.Image]{}
	} else {
		c.img.GPU.Source = weak.Make(b.img)
	}
	c.img.Changed()
}

type webGPUCanvasEffect struct{ canvas *WebGPUCanvas }

func (m webGPUCanvasEffect) EffectMargin() Point { return Point{} }

func (m webGPUCanvasEffect) PaintEffect(p *Painter, box Rect, content *Bitmap, _ Point) {
	m.canvas.SetSource(content)
	srcW, srcH := m.canvas.Size()
	src := Rect{W: float32(srcW), H: float32(srcH)}
	p.ImageRegion(m.canvas.Bitmap(), src, box, ImageOptions{})
}

// WebGPUTexture is an application-owned WebGPU texture that MyGo samples
// as an image. The texture must belong to the renderer's device and use
// premultiplied RGBA8 in MyGo's row order. MyGo creates only a texture
// view; it never uploads or releases the application's texture.
//
// Commands submitted through the device's default queue are ordered with
// MyGo's frame. A producer on another queue must submit before MyGo draws
// the image.
type WebGPUTexture struct {
	img   *scene.Image
	bm    *Bitmap
	owner any
}

// NewWebGPUTexture returns a w×h image showing texture on device. owner is
// retained with the image.
func NewWebGPUTexture(w, h int, device *wgpu.Device, texture *wgpu.Texture, owner any) *WebGPUTexture {
	w, h = max(w, 1), max(h, 1)
	img := scene.NewExternalObjectImage(w, h, device, texture, owner)
	return &WebGPUTexture{
		img: img, bm: &Bitmap{img: img, w: w, h: h}, owner: owner,
	}
}

// Bitmap returns the texture as a bitmap for Image, Painter.Image and
// Painter.ImageRegion.
func (t *WebGPUTexture) Bitmap() *Bitmap { return t.bm }

// Size returns the image's size in pixels. It must match the texture.
func (t *WebGPUTexture) Size() (w, h int) { return t.img.W, t.img.H }

// SetTexture publishes another texture and owner for the same image. The
// size and device must stay the same.
func (t *WebGPUTexture) SetTexture(device *wgpu.Device, texture *wgpu.Texture, owner any) {
	if t.img.External == nil {
		return
	}
	if t.img.External.KeepAlive != nil {
		t.img.External.Retired = append(t.img.External.Retired, t.img.External.KeepAlive)
	}
	t.img.External.DeviceObject = device
	t.img.External.TextureObject = texture
	t.img.External.KeepAlive = owner
	t.owner = owner
	t.img.External.OwnerVersion++
	t.img.Changed()
}

// Invalidate marks externally changed texture contents for another frame.
func (t *WebGPUTexture) Invalidate() { t.img.Changed() }

var _ GPUTexture = (*WebGPUCanvas)(nil)
var _ GPUTexture = (*WebGPUTexture)(nil)
