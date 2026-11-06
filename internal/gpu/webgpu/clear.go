package webgpu

import (
	_ "embed"
	"fmt"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu"
)

//go:embed clear.wgsl
var clearShaderSource string

func (r *Renderer) initClear() error {
	shader, err := r.gpu.device.CreateShaderModuleWGSL(clearShaderSource)
	if err != nil {
		return fmt.Errorf("webgpu: clear shader: %w", err)
	}
	r.clearShader = shader
	pipe, err := r.gpu.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "MyGo damage clear",
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "clear_vs",
		},
		Primitive: wgpu.PrimitiveState{
			Topology: wgpu.PrimitiveTopology(5),
			CullMode: wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "clear_fs",
			Targets: []wgpu.ColorTargetState{{
				Format:    r.format,
				WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("webgpu: clear pipeline: %w", err)
	}
	r.clearPipe = pipe
	r.clearLayout = pipe.GetBindGroupLayout(0)
	if r.clearLayout == nil {
		return fmt.Errorf("webgpu: clear pipeline has no bind group layout")
	}
	return nil
}

func (r *Renderer) clearPart(
	encoder *wgpu.CommandEncoder,
	view *wgpu.TextureView,
	part *gpu.Part,
	width, height int,
	clear [4]float32,
) error {
	if part == nil || len(part.Rects) == 0 {
		return nil
	}
	buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo damage clear color",
		Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
		Size:  16,
	})
	if err != nil {
		return fmt.Errorf("webgpu: clear color buffer: %w", err)
	}
	if err := r.gpu.queue.WriteBuffer(buf, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&clear[0])), 16,
	)); err != nil {
		buf.Release()
		return fmt.Errorf("webgpu: write clear color: %w", err)
	}
	bind, err := r.gpu.device.CreateBindGroupSimple(r.clearLayout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, buf, 0, 16),
	})
	if err != nil {
		buf.Release()
		return fmt.Errorf("webgpu: clear bind group: %w", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "MyGo damage clear pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpLoad, StoreOp: wgpu.StoreOpStore,
		}},
	})
	if err != nil {
		bind.Release()
		buf.Release()
		return fmt.Errorf("webgpu: begin clear pass: %w", err)
	}
	pass.SetPipeline(r.clearPipe)
	pass.SetBindGroup(0, bind, nil)
	for _, rect := range part.Rects {
		left := max(rect.Left, 0)
		top := max(rect.Top, 0)
		right := min(rect.Right, int32(width))
		bottom := min(rect.Bottom, int32(height))
		if right <= left || bottom <= top {
			continue
		}
		pass.SetScissorRect(uint32(left), uint32(top), uint32(right-left), uint32(bottom-top))
		pass.Draw(4, 1, 0, 0)
	}
	pass.End()
	pass.Release()
	r.pending = append(r.pending, func() {
		bind.Release()
		buf.Release()
	})
	return nil
}
