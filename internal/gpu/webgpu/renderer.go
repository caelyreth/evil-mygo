package webgpu

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/scene"
)

const (
	instanceSize    = unsafe.Sizeof(gpu.Instance{})
	initialCapacity = 256
)

// Renderer draws WebGPU scenes. Shared pipeline resources are reused across
// the root frame and every nested native UI canvas; each target owns the
// instance buffer and atlas bindings its scene draws with.
type Renderer struct {
	gpu     *GPU
	format  wgpu.TextureFormat
	shader  *wgpu.ShaderModule
	layout  *wgpu.BindGroupLayout
	pipe    *wgpu.RenderPipeline
	sampler *wgpu.Sampler

	defaultImage *textureEntry
	defaultMask  *textureEntry
	defaultColor *textureEntry
	defaultClip  *textureEntry

	root     drawState
	canvases map[uint64]*canvasEntry
	images   map[uintptr]*textureEntry
	frame    uint64
}

// drawState is the per-target state a scene draws with. Keeping it apart
// from Renderer lets a canvas render before the root without replacing the
// root's instances, uniforms, atlases or bind groups.
type drawState struct {
	builder  gpu.Builder
	globals  *wgpu.Buffer
	instBuf  *wgpu.Buffer
	capacity int
	instRaw  []byte
	globRaw  [16]byte
	stopsBuf *wgpu.Buffer
	stopsCap int
	mask     *textureEntry
	color    *textureEntry
	binds    map[bindKey]*wgpu.BindGroup
}

type canvasEntry struct {
	tex       *textureEntry
	state     drawState
	version   uint64
	lastFrame uint64
}

// NewRenderer creates a scene renderer for targets of format.
func NewRenderer(g *GPU, format wgpu.TextureFormat) (*Renderer, error) {
	if g == nil || g.device == nil {
		return nil, errors.New("webgpu: renderer needs an open GPU")
	}
	r := &Renderer{gpu: g, format: format}
	var err error
	if r.shader, err = g.device.CreateShaderModuleWGSL(sceneShader); err != nil {
		r.Release()
		return nil, fmt.Errorf("webgpu: scene shader: %w", err)
	}
	blend := &wgpu.BlendState{
		Color: wgpu.BlendComponent{
			Operation: gputypes.BlendOperationAdd,
			SrcFactor: gputypes.BlendFactorOne,
			DstFactor: gputypes.BlendFactorOneMinusSrcAlpha,
		},
		Alpha: wgpu.BlendComponent{
			Operation: gputypes.BlendOperationAdd,
			SrcFactor: gputypes.BlendFactorOne,
			DstFactor: gputypes.BlendFactorOneMinusSrcAlpha,
		},
	}
	r.pipe, err = g.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "MyGo scene",
		Vertex: wgpu.VertexState{
			Module: r.shader, EntryPoint: "vs",
		},
		Primitive: wgpu.PrimitiveState{
			// go-webgpu v0.5.5 passes PrimitiveTopology without the
			// gputypes-to-native conversion that it applies to other
			// enums. Native TriangleStrip is 5.
			Topology:  wgpu.PrimitiveTopology(5),
			FrontFace: wgpu.FrontFaceCCW,
			CullMode:  wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1},
		Fragment: &wgpu.FragmentState{
			Module: r.shader, EntryPoint: "fs",
			Targets: []wgpu.ColorTargetState{{
				Format: format, Blend: blend, WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		r.Release()
		return nil, fmt.Errorf("webgpu: scene pipeline: %w", err)
	}
	if r.layout = r.pipe.GetBindGroupLayout(0); r.layout == nil {
		r.Release()
		return nil, errors.New("webgpu: scene pipeline has no bind group layout")
	}
	if r.sampler, err = g.device.CreateLinearSampler(); err != nil {
		r.Release()
		return nil, fmt.Errorf("webgpu: scene sampler: %w", err)
	}
	if r.defaultImage, err = r.solidTexture("MyGo white", wgpu.TextureFormatRGBA8Unorm, [4]byte{255, 255, 255, 255}); err != nil {
		r.Release()
		return nil, err
	}
	if r.defaultMask, err = r.solidTexture("MyGo empty mask", wgpu.TextureFormatR8Unorm, [4]byte{0}); err != nil {
		r.Release()
		return nil, err
	}
	if r.defaultColor, err = r.solidTexture("MyGo empty color", wgpu.TextureFormatRGBA8Unorm, [4]byte{}); err != nil {
		r.Release()
		return nil, err
	}
	if r.defaultClip, err = r.solidTexture("MyGo empty clip", wgpu.TextureFormatRGBA8Unorm, [4]byte{255, 255, 255, 255}); err != nil {
		r.Release()
		return nil, err
	}
	r.images = map[uintptr]*textureEntry{}
	r.canvases = map[uint64]*canvasEntry{}
	r.root.reset(r, r.defaultMask, r.defaultColor)
	if err := r.root.ensureInstances(r, initialCapacity); err != nil {
		r.Release()
		return nil, err
	}
	return r, nil
}

func (s *drawState) reset(r *Renderer, mask, color *textureEntry) {
	s.mask, s.color = mask, color
	s.binds = map[bindKey]*wgpu.BindGroup{}
}

// Render encodes s into view. Clear is the scene background.
func (r *Renderer) Render(s *scene.Scene, encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	if r == nil || r.pipe == nil {
		return errors.New("webgpu: renderer is released")
	}
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	r.frame++
	if err := r.renderScene(s, encoder, view, &r.root); err != nil {
		return err
	}
	r.sweepCanvases()
	return nil
}

func (r *Renderer) renderScene(s *scene.Scene, encoder *wgpu.CommandEncoder, view *wgpu.TextureView, st *drawState) error {
	if err := r.syncAtlas(s.MaskAtlas, wgpu.TextureFormatR8Unorm, &st.mask); err != nil {
		return err
	}
	if err := r.syncAtlas(s.ColorAtlas, wgpu.TextureFormatRGBA8Unorm, &st.color); err != nil {
		return err
	}
	var canvasErr error
	s.EachCanvas(func(img *scene.Image) {
		if canvasErr == nil {
			canvasErr = r.drawCanvas(img, encoder)
		}
	})
	if canvasErr != nil {
		return canvasErr
	}

	var buildErr error
	st.builder.Build(s, func(img *scene.Image) uintptr {
		if buildErr != nil {
			return 0
		}
		token, err := r.syncImage(img)
		if err != nil {
			buildErr = err
			return 0
		}
		return token
	})
	if buildErr != nil {
		return buildErr
	}
	if err := supported(st.builder.Batches, st.builder.Instances); err != nil {
		return err
	}
	if err := st.ensureInstances(r, len(st.builder.Instances)); err != nil {
		return err
	}
	if err := st.writeUniforms(r, s); err != nil {
		return err
	}

	clear := s.Clear
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "MyGo scene pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{
				R: float64(clear.R) / 255,
				G: float64(clear.G) / 255,
				B: float64(clear.B) / 255,
				A: float64(clear.A) / 255,
			},
		}},
	})
	if err != nil {
		return fmt.Errorf("webgpu: begin scene pass: %w", err)
	}
	defer pass.Release()
	if len(st.builder.Instances) > 0 {
		pass.SetPipeline(r.pipe)
		for _, batch := range st.builder.Batches {
			if batch.Count == 0 {
				continue
			}
			image := r.defaultImage.view
			if batch.Image != 0 {
				t := r.texture(batch.Image)
				if t == nil {
					return fmt.Errorf("webgpu: batch references unknown image %#x", batch.Image)
				}
				image = t.view
			}
			clip := r.defaultClip.view
			if batch.ClipMask != 0 {
				t := r.texture(batch.ClipMask)
				if t == nil {
					return fmt.Errorf("webgpu: batch references unknown clip mask %#x", batch.ClipMask)
				}
				clip = t.view
			}
			bind, err := st.bindFor(r, image, clip)
			if err != nil {
				return fmt.Errorf("webgpu: scene bind group: %w", err)
			}
			pass.SetBindGroup(0, bind, nil)
			pass.SetScissorRect(
				uint32(max(batch.Scissor.Left, 0)),
				uint32(max(batch.Scissor.Top, 0)),
				uint32(max(batch.Scissor.Right-batch.Scissor.Left, 0)),
				uint32(max(batch.Scissor.Bottom-batch.Scissor.Top, 0)),
			)
			pass.Draw(4, uint32(batch.Count), 0, uint32(batch.Start))
		}
	}
	pass.End()
	return nil
}

func (s *drawState) writeUniforms(r *Renderer, sc *scene.Scene) error {
	if len(s.builder.Instances) == 0 {
		return nil
	}
	src := unsafe.Slice(
		(*byte)(unsafe.Pointer(&s.builder.Instances[0])),
		len(s.builder.Instances)*int(instanceSize),
	)
	if cap(s.instRaw) < len(src) {
		s.instRaw = make([]byte, len(src))
	}
	s.instRaw = s.instRaw[:len(src)]
	copy(s.instRaw, src)
	if err := r.gpu.queue.WriteBuffer(s.instBuf, 0, s.instRaw); err != nil {
		return fmt.Errorf("webgpu: write scene instances: %w", err)
	}
	globals := (*[4]float32)(unsafe.Pointer(&s.globRaw[0]))
	globals[0], globals[1] = float32(sc.Width), float32(sc.Height)
	if err := r.gpu.queue.WriteBuffer(s.globals, 0, s.globRaw[:]); err != nil {
		return fmt.Errorf("webgpu: write scene globals: %w", err)
	}
	if err := s.writeStops(r); err != nil {
		return err
	}
	return nil
}

func (s *drawState) writeStops(r *Renderer) error {
	n := len(s.builder.Stops)
	texels := max((n+3)/4, 1)
	if texels > s.stopsCap {
		buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "MyGo scene stops",
			Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
			Size:  uint64(texels) * 16,
		})
		if err != nil {
			return fmt.Errorf("webgpu: scene stops buffer: %w", err)
		}
		if s.stopsBuf != nil {
			s.stopsBuf.Release()
		}
		s.forgetBindings()
		s.stopsBuf, s.stopsCap = buf, texels
	}
	if n == 0 {
		var zero [4]float32
		return r.gpu.queue.WriteBuffer(s.stopsBuf, 0, unsafe.Slice(
			(*byte)(unsafe.Pointer(&zero[0])), unsafe.Sizeof(zero),
		))
	}
	return r.gpu.queue.WriteBuffer(s.stopsBuf, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&s.builder.Stops[0])),
		n*4,
	))
}

func (r *Renderer) drawCanvas(img *scene.Image, encoder *wgpu.CommandEncoder) error {
	entry := r.canvases[img.ID()]
	if entry == nil {
		entry = &canvasEntry{}
		entry.state.reset(r, r.defaultMask, r.defaultColor)
		r.canvases[img.ID()] = entry
	}
	entry.lastFrame = r.frame
	sizeChanged := entry.tex == nil || entry.tex.w != img.W || entry.tex.h != img.H
	if sizeChanged {
		next, err := r.createRenderTexture("MyGo canvas", wgpu.TextureFormatRGBA8Unorm, img.W, img.H)
		if err != nil {
			return fmt.Errorf("webgpu: canvas texture: %w", err)
		}
		if entry.tex != nil {
			entry.tex.Release()
			r.forgetBindings()
		}
		entry.tex = next
	}
	if !sizeChanged && entry.version == img.Version() {
		return nil
	}
	view, err := entry.tex.tex.CreateView(nil)
	if err != nil {
		return fmt.Errorf("webgpu: canvas view: %w", err)
	}
	defer view.Release()
	img.Scene.Width, img.Scene.Height = img.W, img.H
	if err := r.renderScene(img.Scene, encoder, view, &entry.state); err != nil {
		return fmt.Errorf("webgpu: draw canvas: %w", err)
	}
	entry.version = img.Version()
	return nil
}

func (r *Renderer) sweepCanvases() {
	removed := false
	for id, entry := range r.canvases {
		if entry.lastFrame == r.frame {
			continue
		}
		if entry.tex != nil {
			entry.tex.Release()
		}
		entry.state.release(r)
		delete(r.canvases, id)
		removed = true
	}
	if removed {
		r.forgetBindings()
	}
}

func supported(batches []gpu.Batch, instances []gpu.Instance) error {
	for _, b := range batches {
		if b.Effect != nil || b.Shader != nil || b.Hole {
			return errors.New("webgpu: scene effect or hole not ported yet")
		}
	}
	for _, in := range instances {
		kind := in.Params[0]
		switch {
		case kind == 0:
			// Fills support every paint, border and corner path.
		case kind >= 1 && kind < 2:
			// Shadows support blur, cast boxes, inset shadows and
			// continuous corners.
		case kind >= 2 && kind <= 5:
			// Glyph, image and color-glyph paths are ported.
		default:
			return errors.New("webgpu: scene instance kind is not ported yet")
		}
	}
	return nil
}

func (s *drawState) ensureInstances(r *Renderer, n int) error {
	n = max(n, 1)
	if n <= s.capacity {
		return nil
	}
	capacity := max(n, s.capacity*2, initialCapacity)
	buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo scene instances",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size:  uint64(capacity) * uint64(instanceSize),
	})
	if err != nil {
		return fmt.Errorf("webgpu: scene instances buffer: %w", err)
	}
	if s.instBuf != nil {
		s.instBuf.Release()
	}
	s.forgetBindings()
	s.instBuf, s.capacity = buf, capacity
	if s.globals == nil {
		s.globals, err = r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "MyGo scene globals",
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
			Size:  16,
		})
		if err != nil {
			return fmt.Errorf("webgpu: scene globals buffer: %w", err)
		}
	}
	return nil
}

func (s *drawState) release(r *Renderer) {
	s.forgetBindings()
	s.builder.Release()
	if s.mask != nil && s.mask != r.defaultMask {
		s.mask.Release()
	}
	if s.color != nil && s.color != r.defaultColor {
		s.color.Release()
	}
	if s.instBuf != nil {
		s.instBuf.Release()
	}
	if s.globals != nil {
		s.globals.Release()
	}
	if s.stopsBuf != nil {
		s.stopsBuf.Release()
	}
	s.mask, s.color, s.instBuf, s.globals, s.stopsBuf = nil, nil, nil, nil, nil
	s.capacity, s.stopsCap, s.instRaw, s.binds = 0, 0, nil, nil
}

func (r *Renderer) texture(token uintptr) *textureEntry {
	if t := r.images[token]; t != nil {
		return t
	}
	if c := r.canvases[uint64(token)]; c != nil {
		return c.tex
	}
	return nil
}

func (r *Renderer) forgetBindings() {
	r.root.forgetBindings()
	for _, c := range r.canvases {
		c.state.forgetBindings()
	}
}

// Release frees the renderer's WebGPU resources.
func (r *Renderer) Release() {
	if r == nil {
		return
	}
	for _, c := range r.canvases {
		if c.tex != nil {
			c.tex.Release()
		}
		c.state.release(r)
	}
	r.canvases = nil
	r.root.release(r)
	for _, t := range r.images {
		t.Release()
	}
	r.images = nil
	if r.sampler != nil {
		r.sampler.Release()
		r.sampler = nil
	}
	if r.defaultImage != nil {
		r.defaultImage.Release()
		r.defaultImage = nil
	}
	if r.defaultMask != nil {
		r.defaultMask.Release()
		r.defaultMask = nil
	}
	if r.defaultColor != nil {
		r.defaultColor.Release()
		r.defaultColor = nil
	}
	if r.defaultClip != nil {
		r.defaultClip.Release()
		r.defaultClip = nil
	}
	if r.pipe != nil {
		r.pipe.Release()
		r.pipe = nil
	}
	if r.layout != nil {
		r.layout.Release()
		r.layout = nil
	}
	if r.shader != nil {
		r.shader.Release()
		r.shader = nil
	}
}
