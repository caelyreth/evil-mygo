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

// Renderer draws the growing WebGPU portion of MyGo's scene language. The
// first stage handles solid rounded fills, projective transforms, image
// batches and grayscale mask glyphs. It rejects features not ported yet
// instead of silently dropping them.
type Renderer struct {
	gpu      *GPU
	format   wgpu.TextureFormat
	shader   *wgpu.ShaderModule
	layout   *wgpu.BindGroupLayout
	pipe     *wgpu.RenderPipeline
	globals  *wgpu.Buffer
	instBuf  *wgpu.Buffer
	sampler  *wgpu.Sampler
	capacity int
	builder  gpu.Builder
	instRaw  []byte
	globRaw  [16]byte

	defaultImage *textureEntry
	defaultMask  *textureEntry
	defaultColor *textureEntry
	mask         *textureEntry
	color        *textureEntry
	images       map[uintptr]*textureEntry
	binds        map[*wgpu.TextureView]*wgpu.BindGroup
}

// NewRenderer creates a scene renderer for targets of format.
func NewRenderer(g *GPU, format wgpu.TextureFormat) (*Renderer, error) {
	if g == nil || g.device == nil {
		return nil, errors.New("webgpu: renderer needs an open GPU")
	}
	r := &Renderer{gpu: g, format: format}
	r.builder.Opaque = true
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
	if r.globals, err = g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo scene globals",
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		Size:  16,
	}); err != nil {
		r.Release()
		return nil, fmt.Errorf("webgpu: scene globals buffer: %w", err)
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
	r.mask, r.color = r.defaultMask, r.defaultColor
	r.images = map[uintptr]*textureEntry{}
	r.binds = map[*wgpu.TextureView]*wgpu.BindGroup{}
	if err := r.ensureInstances(initialCapacity); err != nil {
		r.Release()
		return nil, err
	}
	return r, nil
}

// Render encodes scene into view. Clear is the scene background.
func (r *Renderer) Render(s *scene.Scene, encoder *wgpu.CommandEncoder, view *wgpu.TextureView) error {
	if r == nil || r.pipe == nil {
		return errors.New("webgpu: renderer is released")
	}
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return nil
	}
	if err := r.syncAtlas(s.MaskAtlas, wgpu.TextureFormatR8Unorm, &r.mask, &r.defaultMask); err != nil {
		return err
	}
	if err := r.syncAtlas(s.ColorAtlas, wgpu.TextureFormatRGBA8Unorm, &r.color, &r.defaultColor); err != nil {
		return err
	}
	var buildErr error
	r.builder.Build(s, func(img *scene.Image) uintptr {
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
	if err := supported(r.builder.Batches, r.builder.Instances); err != nil {
		return err
	}
	if err := r.ensureInstances(len(r.builder.Instances)); err != nil {
		return err
	}
	if len(r.builder.Instances) > 0 {
		src := unsafe.Slice(
			(*byte)(unsafe.Pointer(&r.builder.Instances[0])),
			len(r.builder.Instances)*int(instanceSize),
		)
		if cap(r.instRaw) < len(src) {
			r.instRaw = make([]byte, len(src))
		}
		r.instRaw = r.instRaw[:len(src)]
		copy(r.instRaw, src)
		if err := r.gpu.queue.WriteBuffer(r.instBuf, 0, r.instRaw); err != nil {
			return fmt.Errorf("webgpu: write scene instances: %w", err)
		}
		globals := (*[4]float32)(unsafe.Pointer(&r.globRaw[0]))
		globals[0], globals[1] = float32(s.Width), float32(s.Height)
		if err := r.gpu.queue.WriteBuffer(r.globals, 0, r.globRaw[:]); err != nil {
			return fmt.Errorf("webgpu: write scene globals: %w", err)
		}
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
	if len(r.builder.Instances) > 0 {
		pass.SetPipeline(r.pipe)
		for _, batch := range r.builder.Batches {
			if batch.Count == 0 {
				continue
			}
			image := r.defaultImage.view
			if batch.Image != 0 {
				t := r.images[batch.Image]
				if t == nil {
					return fmt.Errorf("webgpu: batch references unknown image %#x", batch.Image)
				}
				image = t.view
			}
			bind, err := r.bindFor(image)
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

func supported(batches []gpu.Batch, instances []gpu.Instance) error {
	for _, b := range batches {
		if b.ClipMask != 0 || b.Effect != nil || b.Shader != nil || b.Hole {
			return errors.New("webgpu: scene feature not ported yet")
		}
	}
	for _, in := range instances {
		kind := in.Params[0]
		switch {
		case kind == 0:
			if in.Params[2] != 0 || in.UV != [4]float32{} {
				return errors.New("webgpu: gradients, borders and dashed fills are not ported yet")
			}
			if in.Radii[0] < 0 || in.Radii[1] < 0 || in.Radii[2] < 0 || in.Radii[3] < 0 {
				return errors.New("webgpu: continuous corners are not ported yet")
			}
		case kind >= 2 && kind <= 5:
			if in.Params[2] != 0 {
				return errors.New("webgpu: textured gradients are not ported yet")
			}
		default:
			return errors.New("webgpu: scene instance kind is not ported yet")
		}
		if in.ClipRadii[0] < 0 || in.ClipRadii[1] < 0 || in.ClipRadii[2] < 0 || in.ClipRadii[3] < 0 {
			return errors.New("webgpu: continuous clip masks are not ported yet")
		}
	}
	return nil
}

func (r *Renderer) ensureInstances(n int) error {
	n = max(n, 1)
	if n <= r.capacity {
		return nil
	}
	capacity := max(n, r.capacity*2, initialCapacity)
	buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo scene instances",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size:  uint64(capacity) * uint64(instanceSize),
	})
	if err != nil {
		return fmt.Errorf("webgpu: scene instances buffer: %w", err)
	}
	if r.instBuf != nil {
		r.instBuf.Release()
	}
	r.forgetBindings()
	r.instBuf, r.capacity = buf, capacity
	return nil
}

// Release frees the renderer's WebGPU resources.
func (r *Renderer) Release() {
	if r == nil {
		return
	}
	r.builder.Release()
	r.forgetBindings()
	if r.sampler != nil {
		r.sampler.Release()
		r.sampler = nil
	}
	for _, t := range r.images {
		t.Release()
	}
	r.images = nil
	if r.mask != r.defaultMask {
		r.mask.Release()
	}
	if r.color != r.defaultColor {
		r.color.Release()
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
	if r.instBuf != nil {
		r.instBuf.Release()
		r.instBuf = nil
	}
	if r.globals != nil {
		r.globals.Release()
		r.globals = nil
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
