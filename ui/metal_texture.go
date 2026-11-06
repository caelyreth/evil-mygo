package ui

import (
	"unsafe"

	"github.com/egoist/mygo/internal/scene"
)

// MetalTextureSync orders MyGo's reads of an application-owned
// MTLTexture with its producer. Wait runs once, before the texture is
// first sampled in a frame's command buffer; Signal runs after all of
// that frame's sampling has been encoded. Both run on the main thread
// with the renderer's device, command buffer and texture.
//
// The callbacks may encode an MTLSharedEvent wait/signal or a fence into
// the command buffer. They must not commit or wait on it. A Wait error
// fails the frame. Without synchronization callbacks, the producer must
// guarantee that the texture is ready before MyGo commits the frame.
type MetalTextureSync struct {
	Wait   func(device, commandBuffer, texture unsafe.Pointer) error
	Signal func(device, commandBuffer, texture unsafe.Pointer) error
}

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
	img  *scene.Image
	bm   *Bitmap
	sync MetalTextureSync
}

// NewMetalTexture returns a w×h image showing texture, an MTLTexture on
// device. The application must keep the native texture alive; owner, when
// non-nil, is retained by the image as well.
func NewMetalTexture(w, h int, device, texture unsafe.Pointer, owner any) *MetalTexture {
	return NewMetalTextureWithSync(w, h, device, texture, owner, MetalTextureSync{})
}

// NewMetalTextureWithSync is NewMetalTexture with explicit producer
// synchronization. See MetalTextureSync.
func NewMetalTextureWithSync(w, h int, device, texture unsafe.Pointer, owner any, sync MetalTextureSync) *MetalTexture {
	w, h = max(w, 1), max(h, 1)
	img := scene.NewExternalImage(w, h, device, texture, owner)
	img.External.Sync = sceneExternalSync(sync)
	return &MetalTexture{img: img, bm: &Bitmap{img: img, w: w, h: h}, sync: sync}
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
	t.SetTextureWithSync(device, texture, owner, t.sync)
}

// SetTextureWithSync is SetTexture and SetSync together.
func (t *MetalTexture) SetTextureWithSync(device, texture unsafe.Pointer, owner any, sync MetalTextureSync) {
	if t.img.External == nil {
		return
	}
	if t.img.External.KeepAlive != nil {
		t.img.External.Retired = append(t.img.External.Retired, t.img.External.KeepAlive)
	}
	t.img.External.Device, t.img.External.Texture, t.img.External.KeepAlive = device, texture, owner
	t.sync = sync
	t.img.External.Sync = sceneExternalSync(sync)
	t.img.External.OwnerVersion++
	t.img.Changed()
}

// SetSync changes the producer synchronization used by later frames
// without changing the texture.
func (t *MetalTexture) SetSync(sync MetalTextureSync) {
	if t.img.External == nil {
		return
	}
	t.sync = sync
	t.img.External.Sync = sceneExternalSync(sync)
}

// Invalidate marks an externally changed texture for another frame.
func (t *MetalTexture) Invalidate() { t.img.Changed() }

func sceneExternalSync(sync MetalTextureSync) scene.ExternalTextureSync {
	return scene.ExternalTextureSync{Wait: sync.Wait, Signal: sync.Signal}
}
