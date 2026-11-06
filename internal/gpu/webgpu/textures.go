package webgpu

import (
	"errors"
	"fmt"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

type textureEntry struct {
	tex        *wgpu.Texture
	view       *wgpu.TextureView
	w, h       int
	format     wgpu.TextureFormat
	generation uint64
	version    uint64
}

type bindKey struct {
	image *wgpu.TextureView
	clip  *wgpu.TextureView
}

func (t *textureEntry) Release() {
	if t == nil {
		return
	}
	if t.view != nil {
		t.view.Release()
		t.view = nil
	}
	if t.tex != nil {
		t.tex.Release()
		t.tex = nil
	}
}

func (r *Renderer) createTexture(label string, format wgpu.TextureFormat, width, height int) (*textureEntry, error) {
	width, height = max(width, 1), max(height, 1)
	tex, err := r.gpu.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         label,
		Usage:         wgpu.TextureUsageTextureBinding | wgpu.TextureUsageCopyDst,
		Dimension:     wgpu.TextureDimension2D,
		Size:          wgpu.Extent3D{Width: uint32(width), Height: uint32(height), DepthOrArrayLayers: 1},
		Format:        format,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		return nil, err
	}
	view, err := tex.CreateView(nil)
	if err != nil {
		tex.Release()
		return nil, err
	}
	return &textureEntry{tex: tex, view: view, w: width, h: height, format: format}, nil
}

func (r *Renderer) uploadTexture(t *textureEntry, pix []byte, bpp int) error {
	if t == nil || len(pix) < t.w*t.h*bpp {
		return errors.New("webgpu: texture pixels are incomplete")
	}
	rowBytes := t.w * bpp
	stride := (rowBytes + 255) &^ 255
	data := make([]byte, stride*t.h)
	for y := range t.h {
		copy(data[y*stride:y*stride+rowBytes], pix[y*rowBytes:(y+1)*rowBytes])
	}
	return r.gpu.queue.WriteTexture(
		&wgpu.ImageCopyTexture{Texture: t.tex, Aspect: wgpu.TextureAspectAll},
		data,
		&wgpu.ImageDataLayout{Offset: 0, BytesPerRow: uint32(stride), RowsPerImage: uint32(t.h)},
		&wgpu.Extent3D{Width: uint32(t.w), Height: uint32(t.h), DepthOrArrayLayers: 1},
	)
}

func (r *Renderer) solidTexture(label string, format wgpu.TextureFormat, rgba [4]byte) (*textureEntry, error) {
	t, err := r.createTexture(label, format, 1, 1)
	if err != nil {
		return nil, err
	}
	var pix []byte
	if format == wgpu.TextureFormatR8Unorm {
		pix = []byte{rgba[0]}
	} else {
		pix = rgba[:]
	}
	if err := r.uploadTexture(t, pix, len(pix)); err != nil {
		t.Release()
		return nil, err
	}
	return t, nil
}

func (r *Renderer) syncAtlas(a *scene.Atlas, format wgpu.TextureFormat, current **textureEntry, fallback **textureEntry) error {
	if a == nil || a.W <= 0 || a.H <= 0 {
		if *current != *fallback {
			(*current).Release()
			r.forgetBindings()
		}
		*current = *fallback
		return nil
	}
	t := *current
	if t == *fallback {
		t = nil
	}
	changed := false
	if t == nil || t.w != a.W || t.h != a.H || t.format != format {
		oldView := (*wgpu.TextureView)(nil)
		if t != nil {
			oldView = t.view
		}
		next, err := r.createTexture("MyGo atlas", format, a.W, a.H)
		if err != nil {
			return err
		}
		if t != nil {
			t.Release()
		}
		t = next
		*current = t
		if oldView != nil {
			r.forgetBindings()
		}
		changed = true
	}
	if changed || t.generation != a.Generation() || t.version != a.Version() {
		bpp := 1
		if format == wgpu.TextureFormatRGBA8Unorm {
			bpp = 4
		}
		if err := r.uploadTexture(t, a.Pix, bpp); err != nil {
			return fmt.Errorf("webgpu: upload atlas: %w", err)
		}
		t.generation, t.version = a.Generation(), a.Version()
	}
	return nil
}

func (r *Renderer) syncImage(img *scene.Image) (uintptr, error) {
	if img == nil {
		return 0, nil
	}
	if img.Scene != nil || img.GPU != nil || img.External != nil {
		return 0, errors.New("webgpu: canvas and external images are not ported yet")
	}
	token := uintptr(img.ID())
	t := r.images[token]
	if t == nil {
		var err error
		t, err = r.createTexture("MyGo image", wgpu.TextureFormatRGBA8Unorm, img.W, img.H)
		if err != nil {
			return 0, err
		}
		r.images[token] = t
	}
	if t.w != img.W || t.h != img.H {
		t.Release()
		next, err := r.createTexture("MyGo image", wgpu.TextureFormatRGBA8Unorm, img.W, img.H)
		if err != nil {
			delete(r.images, token)
			return 0, err
		}
		*t = *next
		r.forgetBindings()
	}
	if t.version != img.Version() {
		pix := img.Pixels()
		if pix == nil {
			return 0, errors.New("webgpu: image has no pixels")
		}
		if err := r.uploadTexture(t, pix, 4); err != nil {
			return 0, fmt.Errorf("webgpu: upload image: %w", err)
		}
		t.version = img.Version()
		img.Uploaded()
	}
	return token, nil
}

func (r *Renderer) bindFor(image, clip *wgpu.TextureView) (*wgpu.BindGroup, error) {
	if image == nil {
		image = r.defaultImage.view
	}
	if clip == nil {
		clip = r.defaultClip.view
	}
	key := bindKey{image: image, clip: clip}
	if bind := r.binds[key]; bind != nil {
		return bind, nil
	}
	bind, err := r.gpu.device.CreateBindGroupSimple(r.layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, r.globals, 0, 16),
		wgpu.BufferBindingEntry(1, r.instBuf, 0, uint64(r.capacity)*uint64(instanceSize)),
		wgpu.TextureBindingEntry(2, image),
		wgpu.TextureBindingEntry(3, r.mask.view),
		wgpu.TextureBindingEntry(4, r.color.view),
		wgpu.SamplerBindingEntry(5, r.sampler),
		wgpu.TextureBindingEntry(6, clip),
	})
	if err != nil {
		return nil, err
	}
	r.binds[key] = bind
	return bind, nil
}

func (r *Renderer) forgetBindings() {
	for key, bind := range r.binds {
		bind.Release()
		delete(r.binds, key)
	}
}
