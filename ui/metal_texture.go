package ui

import (
	"unsafe"

	"github.com/egoist/mygo/internal/scene"
)

// MetalTexture is an application-owned Metal texture that MyGo samples as
// an image. It is for a producer that already has a texture on the
// renderer's device, such as a game engine or a video decoder. The pixels
// are not uploaded through Go.
//
// The texture must be RGBA8 with premultiplied alpha and the same
// orientation as MyGo images. owner may be a Go wrapper for the native
// object; MyGo keeps it alive with the image. A renderer that is not
// Metal, or a texture from another device, shows nothing.
type MetalTexture struct {
	img *scene.Image
	bm  *Bitmap
}

// NewMetalTexture returns a w×h image showing texture, an MTLTexture on
// device. The application must keep the native texture alive; owner, when
// non-nil, is retained by the image as well.
func NewMetalTexture(w, h int, device, texture unsafe.Pointer, owner any) *MetalTexture {
	w, h = max(w, 1), max(h, 1)
	img := scene.NewExternalImage(w, h, device, texture, owner)
	return &MetalTexture{img: img, bm: &Bitmap{img: img, w: w, h: h}}
}

// Bitmap returns the texture as a bitmap for Image, Painter.Image and
// Painter.ImageRegion.
func (t *MetalTexture) Bitmap() *Bitmap { return t.bm }

// Size returns the image's size in pixels. It must match the native
// texture's size.
func (t *MetalTexture) Size() (w, h int) { return t.img.W, t.img.H }

// SetTexture publishes another native texture and owner for the same
// image, for a producer that recreates its texture. The size must stay the
// same; make a new MetalTexture when it changes.
func (t *MetalTexture) SetTexture(device, texture unsafe.Pointer, owner any) {
	if t.img.External == nil {
		return
	}
	t.img.External.Device, t.img.External.Texture, t.img.External.KeepAlive = device, texture, owner
	t.img.Changed()
}

// Invalidate marks an externally changed texture for another frame.
func (t *MetalTexture) Invalidate() { t.img.Changed() }
