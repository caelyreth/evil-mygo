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
// first stage handles solid rounded fills with projective transforms. It
// rejects features not ported yet instead of silently dropping them.
type Renderer struct {
	gpu      *GPU
	format   wgpu.TextureFormat
	shader   *wgpu.ShaderModule
	layout   *wgpu.BindGroupLayout
	pipe     *wgpu.RenderPipeline
	globals  *wgpu.Buffer
	instBuf  *wgpu.Buffer
	bind     *wgpu.BindGroup
	capacity int
	builder  gpu.Builder
	instRaw  []byte
	globRaw  [16]byte
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
			SrcFactor: gputypes.BlendFactorSrcAlpha,
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
	r.builder.Build(s, func(*scene.Image) uintptr { return 0 })
	if err := supported(r.builder.Batches); err != nil {
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
		pass.SetBindGroup(0, r.bind, nil)
		pass.Draw(4, uint32(len(r.builder.Instances)), 0, 0)
	}
	pass.End()
	return nil
}

func supported(batches []gpu.Batch) error {
	for _, b := range batches {
		if b.Image != 0 || b.ClipMask != 0 || b.Effect != nil || b.Shader != nil || b.Hole {
			return errors.New("webgpu: scene feature not ported yet")
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
	bind, err := r.gpu.device.CreateBindGroupSimple(r.layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, r.globals, 0, 16),
		wgpu.BufferBindingEntry(1, buf, 0, uint64(capacity)*uint64(instanceSize)),
	})
	if err != nil {
		buf.Release()
		return fmt.Errorf("webgpu: scene bind group: %w", err)
	}
	if r.instBuf != nil {
		r.instBuf.Release()
	}
	if r.bind != nil {
		r.bind.Release()
	}
	r.instBuf, r.bind, r.capacity = buf, bind, capacity
	return nil
}

// Release frees the renderer's WebGPU resources.
func (r *Renderer) Release() {
	if r == nil {
		return
	}
	r.builder.Release()
	if r.bind != nil {
		r.bind.Release()
		r.bind = nil
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
