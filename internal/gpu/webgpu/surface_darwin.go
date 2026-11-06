//go:build darwin

package webgpu

import (
	"errors"
	"fmt"

	"github.com/go-webgpu/webgpu/wgpu"
)

// Surface is a wgpu-native surface over an application-owned CAMetalLayer.
// Drawing goes through the WebGPU device and queue.
type Surface struct {
	gpu         *GPU
	surface     *wgpu.Surface
	format      wgpu.TextureFormat
	presentMode wgpu.PresentMode
	alphaMode   wgpu.CompositeAlphaMode
	width       int
	height      int
	suboptimal  bool
}

// NewSurface creates a WebGPU surface from layer, a CAMetalLayer owned by
// the platform host, and chooses a presentation format from the adapter's
// capabilities. The caller keeps the layer alive until Surface.Release.
func (g *GPU) NewSurface(layer uintptr) (*Surface, error) {
	if g == nil || g.instance == nil || g.adapter == nil || g.device == nil {
		return nil, errors.New("webgpu: GPU is not open")
	}
	s, err := g.instance.CreateSurfaceFromMetalLayer(layer)
	if err != nil {
		return nil, fmt.Errorf("webgpu: create surface: %w", err)
	}
	caps, err := s.GetCapabilities(g.adapter)
	if err != nil {
		s.Release()
		return nil, fmt.Errorf("webgpu: surface capabilities: %w", err)
	}
	format, ok := chooseSurfaceFormat(caps.Formats)
	if !ok {
		s.Release()
		return nil, errors.New("webgpu: surface has no RGBA8 presentation format")
	}
	return &Surface{
		gpu: g, surface: s, format: format,
		presentMode: choosePresentMode(caps.PresentModes),
		alphaMode:   chooseAlphaMode(caps.AlphaModes),
	}, nil
}

// Configure sizes and configures the surface for rendering.
func (s *Surface) Configure(width, height int) error {
	if s == nil || s.surface == nil {
		return errors.New("webgpu: surface is released")
	}
	width, height = max(width, 1), max(height, 1)
	if err := s.surface.Configure(s.gpu.device, &wgpu.SurfaceConfiguration{
		Format:      s.format,
		Usage:       wgpu.TextureUsageRenderAttachment,
		Width:       uint32(width),
		Height:      uint32(height),
		AlphaMode:   s.alphaMode,
		PresentMode: s.presentMode,
	}); err != nil {
		return fmt.Errorf("webgpu: configure surface: %w", err)
	}
	s.width, s.height = width, height
	s.suboptimal = false
	return nil
}

// Acquire returns the surface texture for this frame. The caller releases
// tex.Texture after encoding. Suboptimal reports that a resize should
// reconfigure the surface after this frame.
func (s *Surface) Acquire() (tex *wgpu.SurfaceTexture, suboptimal bool, err error) {
	if s == nil || s.surface == nil {
		return nil, false, errors.New("webgpu: surface is released")
	}
	tex, suboptimal, err = s.surface.GetCurrentTexture()
	if err != nil {
		return nil, false, err
	}
	s.suboptimal = suboptimal
	return tex, suboptimal, nil
}

// Present presents the last submitted surface frame.
func (s *Surface) Present() error {
	if s == nil || s.surface == nil {
		return errors.New("webgpu: surface is released")
	}
	return s.surface.Present()
}

// Format is the surface's chosen color format.
func (s *Surface) Format() wgpu.TextureFormat {
	if s == nil {
		return wgpu.TextureFormatUndefined
	}
	return s.format
}

// Size returns the configured pixel size.
func (s *Surface) Size() (width, height int) {
	if s == nil {
		return 0, 0
	}
	return s.width, s.height
}

// Suboptimal reports that the last acquired texture should be followed by
// a reconfigure.
func (s *Surface) Suboptimal() bool { return s != nil && s.suboptimal }

// Release frees the WebGPU surface. The platform host still owns the
// CAMetalLayer it supplied.
func (s *Surface) Release() {
	if s == nil {
		return
	}
	if s.surface != nil {
		s.surface.Release()
		s.surface = nil
	}
}

func chooseSurfaceFormat(formats []wgpu.TextureFormat) (wgpu.TextureFormat, bool) {
	for _, want := range []wgpu.TextureFormat{
		wgpu.TextureFormatBGRA8UnormSrgb,
		wgpu.TextureFormatRGBA8UnormSrgb,
		wgpu.TextureFormatBGRA8Unorm,
		wgpu.TextureFormatRGBA8Unorm,
	} {
		for _, format := range formats {
			if format == want {
				return want, true
			}
		}
	}
	return wgpu.TextureFormatUndefined, false
}

func choosePresentMode(modes []wgpu.PresentMode) wgpu.PresentMode {
	for _, want := range []wgpu.PresentMode{
		wgpu.PresentModeFifo,
		wgpu.PresentModeMailbox,
		wgpu.PresentModeImmediate,
	} {
		for _, mode := range modes {
			if mode == want {
				return want
			}
		}
	}
	return wgpu.PresentModeFifo
}

func chooseAlphaMode(modes []wgpu.CompositeAlphaMode) wgpu.CompositeAlphaMode {
	for _, want := range []wgpu.CompositeAlphaMode{
		wgpu.CompositeAlphaModePremultiplied,
		wgpu.CompositeAlphaModeAuto,
		wgpu.CompositeAlphaModeOpaque,
	} {
		for _, mode := range modes {
			if mode == want {
				return want
			}
		}
	}
	return wgpu.CompositeAlphaModeAuto
}
