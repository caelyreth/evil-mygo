//go:build darwin

package webgpu

import (
	"errors"
	"fmt"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

// NativeRenderer draws MyGo scenes through a wgpu-native surface backed by
// a CAMetalLayer. wgpu-native selects Metal on macOS, so the same WebGPU
// renderer can later run on other platform surfaces without another scene
// implementation.
type NativeRenderer struct {
	gpu      *GPU
	surface  *Surface
	renderer *Renderer
	width    int
	height   int
}

// NewNative creates the macOS presentation renderer for layer, a
// CAMetalLayer owned by the window host.
func NewNative(layer uintptr) (*NativeRenderer, error) {
	if layer == 0 {
		return nil, errors.New("webgpu: no CAMetalLayer")
	}
	g, err := Open()
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*NativeRenderer, error) {
		g.Release()
		return nil, err
	}
	if info := g.AdapterInfo(); info.BackendType != wgpu.BackendTypeMetal {
		return fail(fmt.Errorf("webgpu: macOS adapter is %v, want Metal", info.BackendType))
	}
	surface, err := g.NewSurface(layer)
	if err != nil {
		return fail(err)
	}
	renderer, err := NewRenderer(g, surface.Format())
	if err != nil {
		surface.Release()
		return fail(err)
	}
	return &NativeRenderer{gpu: g, surface: surface, renderer: renderer}, nil
}

// Render draws s into the next surface texture, submits it and presents it.
// The renderer reconfigures the surface when the scene's pixel size changes.
func (r *NativeRenderer) Render(s *scene.Scene) error {
	if r == nil || r.renderer == nil || r.surface == nil {
		return errors.New("webgpu: renderer is released")
	}
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	if r.width != s.Width || r.height != s.Height {
		if err := r.surface.Configure(s.Width, s.Height); err != nil {
			return err
		}
		r.width, r.height = s.Width, s.Height
	}

	surfaceTexture, suboptimal, err := r.surface.Acquire()
	if errors.Is(err, wgpu.ErrSurfaceOccluded) {
		// A minimized or fully covered window has no drawable. The next
		// requested frame acquires one after the window shows again.
		return nil
	}
	if err != nil {
		return fmt.Errorf("webgpu: acquire surface texture: %w", err)
	}
	if surfaceTexture == nil || surfaceTexture.Texture == nil {
		return errors.New("webgpu: surface returned no texture")
	}
	texture := surfaceTexture.Texture
	defer texture.Release()

	view, err := texture.CreateView(nil)
	if err != nil {
		return fmt.Errorf("webgpu: surface texture view: %w", err)
	}
	defer view.Release()
	encoder, err := r.gpu.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{
		Label: "MyGo frame",
	})
	if err != nil {
		return fmt.Errorf("webgpu: create frame encoder: %w", err)
	}
	defer encoder.Release()
	if err := r.renderer.Render(s, encoder, view); err != nil {
		return err
	}
	command, err := encoder.Finish(nil)
	if err != nil {
		return fmt.Errorf("webgpu: finish frame: %w", err)
	}
	if _, err := r.gpu.queue.Submit(command); err != nil {
		command.Release()
		return fmt.Errorf("webgpu: submit frame: %w", err)
	}
	command.Release()
	if err := r.surface.Present(); err != nil {
		return fmt.Errorf("webgpu: present frame: %w", err)
	}
	if suboptimal {
		r.width, r.height = 0, 0
	}
	return nil
}

// Invalidate has no persistent backing store to discard: every WebGPU
// frame is encoded whole.
func (r *NativeRenderer) Invalidate() {}

// FrameInfo describes the active adapter for MYGO_FRAME_STATS.
func (r *NativeRenderer) FrameInfo() string {
	if r == nil || r.gpu == nil {
		return "webgpu"
	}
	info := r.gpu.AdapterInfo()
	return fmt.Sprintf("webgpu/%v %s", info.BackendType, info.Description)
}

// Release frees the scene renderer, surface, device and instance.
func (r *NativeRenderer) Release() {
	if r == nil {
		return
	}
	if r.renderer != nil {
		r.renderer.Release()
		r.renderer = nil
	}
	if r.surface != nil {
		r.surface.Release()
		r.surface = nil
	}
	if r.gpu != nil {
		r.gpu.Release()
		r.gpu = nil
	}
	r.width, r.height = 0, 0
}
