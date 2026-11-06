//go:build darwin && (cgo || nofakecgo)

package webgpu

import (
	"errors"
	"fmt"
	"image"
	"os"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu"
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
	wide     bool
	partial  bool
	held     bool
	diff     scene.Differ
	part     gpu.Part
	content  *textureEntry
	contentW int
	contentH int
}

type nsPoint struct{ X, Y float64 }
type nsSize struct{ Width, Height float64 }
type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

var (
	metalLayerOnce sync.Once
	metalLayerErr  error
	metalDevice    func() uintptr
)

const (
	metalPixelFormatBGRA8Unorm = 80
	nsViewSizable              = 2 | 16
)

// NewNative creates the macOS presentation renderer for view, an NSView
// whose layer host receives a CAMetalLayer. A caller that already owns a
// CAMetalLayer may pass it directly, as the surface tests do.
func NewNative(viewOrLayer uintptr) (*NativeRenderer, error) {
	if viewOrLayer == 0 {
		return nil, errors.New("webgpu: no NSView or CAMetalLayer")
	}
	layer, err := ensureMetalLayer(viewOrLayer)
	if err != nil {
		return nil, err
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
	return &NativeRenderer{
		gpu: g, surface: surface, renderer: renderer, wide: surface.Wide(),
		partial: os.Getenv("MYGO_PARTIAL") == "1",
	}, nil
}

func ensureMetalLayer(viewOrLayer uintptr) (uintptr, error) {
	metalLayerOnce.Do(func() {
		if _, err := purego.Dlopen("/System/Library/Frameworks/QuartzCore.framework/QuartzCore", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			metalLayerErr = fmt.Errorf("webgpu: load QuartzCore: %w", err)
			return
		}
		metal, err := purego.Dlopen("/System/Library/Frameworks/Metal.framework/Metal", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			metalLayerErr = fmt.Errorf("webgpu: load Metal: %w", err)
			return
		}
		purego.RegisterLibFunc(&metalDevice, metal, "MTLCreateSystemDefaultDevice")
	})
	if metalLayerErr != nil {
		return 0, metalLayerErr
	}
	obj := objc.ID(viewOrLayer)
	class := objc.GetClass("CAMetalLayer")
	if class == 0 {
		return 0, errors.New("webgpu: CAMetalLayer is unavailable")
	}
	isLayer := objc.Send[bool](obj, objc.RegisterName("isKindOfClass:"), objc.ID(class))
	if isLayer {
		return viewOrLayer, nil
	}
	parent := objc.Send[objc.ID](obj, objc.RegisterName("layer"))
	if parent == 0 {
		return 0, errors.New("webgpu: NSView has no layer")
	}
	if sublayers := objc.Send[objc.ID](parent, objc.RegisterName("sublayers")); sublayers != 0 {
		for i, count := uint(0), objc.Send[uint](sublayers, objc.RegisterName("count")); i < count; i++ {
			candidate := objc.Send[objc.ID](sublayers, objc.RegisterName("objectAtIndex:"), i)
			if candidate != 0 && objc.Send[bool](candidate, objc.RegisterName("isKindOfClass:"), objc.ID(class)) {
				return uintptr(candidate), nil
			}
		}
	}
	layer := objc.Send[objc.ID](objc.ID(class), objc.RegisterName("alloc"))
	layer = objc.Send[objc.ID](layer, objc.RegisterName("init"))
	if layer == 0 {
		return 0, errors.New("webgpu: cannot create CAMetalLayer")
	}
	device := metalDevice()
	if device == 0 {
		objc.Send[objc.ID](layer, objc.RegisterName("release"))
		return 0, errors.New("webgpu: no Metal device")
	}
	objc.Send[objc.ID](layer, objc.RegisterName("setDevice:"), device)
	objc.Send[objc.ID](layer, objc.RegisterName("setPixelFormat:"), uintptr(metalPixelFormatBGRA8Unorm))
	objc.Send[objc.ID](layer, objc.RegisterName("setFramebufferOnly:"), uintptr(0))
	objc.Send[objc.ID](layer, objc.RegisterName("setOpaque:"), uintptr(0))
	objc.Send[objc.ID](layer, objc.RegisterName("setPresentsWithTransaction:"), uintptr(0))
	objc.Send[objc.ID](layer, objc.RegisterName("setAutoresizingMask:"), uintptr(nsViewSizable))
	bounds := objc.Send[nsRect](obj, objc.RegisterName("bounds"))
	objc.Send[objc.ID](layer, objc.RegisterName("setFrame:"), bounds)
	ifwindow := objc.Send[objc.ID](obj, objc.RegisterName("window"))
	if ifwindow != 0 {
		scale := objc.Send[float64](ifwindow, objc.RegisterName("backingScaleFactor"))
		objc.Send[objc.ID](layer, objc.RegisterName("setContentsScale:"), scale)
	}
	objc.Send[objc.ID](parent, objc.RegisterName("addSublayer:"), layer)
	objc.Send[objc.ID](layer, objc.RegisterName("release"))
	return uintptr(layer), nil
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
	if r.width != s.Width || r.height != s.Height || r.surface.needsConfigure() {
		r.held = false
		r.diff.Invalidate()
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

	var partial *gpu.Part
	var damage []image.Rectangle
	var known bool
	reuse := false
	if r.partial {
		damage, known = r.diff.Diff(s)
		if r.held {
			reuse = known && len(damage) == 0
			if known && r.part.Set(damage, s.Width, s.Height, func(d []image.Rectangle) []image.Rectangle {
				return r.diff.AddBackdrops(s, d)
			}) {
				partial = &r.part
			}
		} else {
			known = false
		}
		if r.content == nil || r.content.w != s.Width || r.content.h != s.Height {
			if err := r.ensureContent(s.Width, s.Height); err != nil {
				return err
			}
		}
	}

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
	var renderErr error
	if r.partial {
		if !reuse {
			if partial != nil {
				renderErr = r.renderer.RenderTargetPart(s, encoder, r.content.tex, r.content.view, partial)
			} else {
				renderErr = r.renderer.RenderTarget(s, encoder, r.content.tex, r.content.view)
			}
		}
		if renderErr == nil {
			renderErr = r.renderer.copyTexture(encoder, r.content.view, view, s.Width, s.Height)
		}
	} else {
		renderErr = r.renderer.RenderTarget(s, encoder, texture, view)
	}
	if renderErr != nil {
		return renderErr
	}
	if r.partial {
		r.diff.Remember(s)
		r.held = true
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
	r.renderer.FinishFrame()
	if err := r.surface.Present(); err != nil {
		return fmt.Errorf("webgpu: present frame: %w", err)
	}
	if suboptimal {
		r.width, r.height = 0, 0
	}
	return nil
}

func (r *NativeRenderer) ensureContent(width, height int) error {
	if r.content != nil && r.contentW == width && r.contentH == height {
		return nil
	}
	next, err := r.renderer.createSurfaceContentTexture("MyGo WebGPU surface content", r.renderer.format, width, height)
	if err != nil {
		return fmt.Errorf("webgpu: surface content: %w", err)
	}
	if r.content != nil {
		r.content.Release()
	}
	r.content, r.contentW, r.contentH = next, width, height
	return nil
}

// Invalidate makes the next frame draw whole, including when the optional
// partial-presentation mode is enabled.
func (r *NativeRenderer) Invalidate() {
	if r == nil {
		return
	}
	r.held = false
	r.diff.Invalidate()
}

// SetWide switches the surface and scene pipelines between 8-bit sRGB and
// RGBA16Float extended-range presentation. The old renderer is released
// only after the device has completed work encoded with its pipelines.
func (r *NativeRenderer) SetWide(on bool) bool {
	if r == nil || r.surface == nil || r.renderer == nil || r.gpu == nil {
		return false
	}
	if r.wide == on {
		return true
	}
	if !r.surface.SetWide(on) {
		return false
	}
	format := r.surface.Format()
	r.gpu.device.Poll(true)
	next, err := NewRenderer(r.gpu, format)
	if err != nil {
		r.surface.SetWide(r.wide)
		return false
	}
	r.renderer.Release()
	r.renderer = next
	r.wide = on
	r.width, r.height = 0, 0
	r.held = false
	r.diff.Invalidate()
	if r.content != nil {
		r.content.Release()
		r.content = nil
	}
	return true
}

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
		if r.content != nil {
			r.content.Release()
			r.content = nil
		}
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
