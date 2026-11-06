package webgpu

import (
	_ "embed"
	"fmt"

	"github.com/go-webgpu/webgpu/wgpu"
)

//go:embed copy.wgsl
var copyShaderSource string

func (r *Renderer) initCopy() error {
	shader, err := r.gpu.device.CreateShaderModuleWGSL(copyShaderSource)
	if err != nil {
		return fmt.Errorf("webgpu: copy shader: %w", err)
	}
	r.copyShader = shader
	pipe, err := r.gpu.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label: "MyGo surface copy",
		Vertex: wgpu.VertexState{
			Module: shader, EntryPoint: "copy_vs",
		},
		Primitive: wgpu.PrimitiveState{
			Topology: wgpu.PrimitiveTopology(5),
			CullMode: wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "copy_fs",
			Targets: []wgpu.ColorTargetState{{
				Format:    r.format,
				WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("webgpu: copy pipeline: %w", err)
	}
	r.copyPipe = pipe
	r.copyLayout = pipe.GetBindGroupLayout(0)
	if r.copyLayout == nil {
		return fmt.Errorf("webgpu: copy pipeline has no bind group layout")
	}
	return nil
}

func (r *Renderer) copyTexture(encoder *wgpu.CommandEncoder, src, dst *wgpu.TextureView, width, height int) error {
	bind, err := r.gpu.device.CreateBindGroupSimple(r.copyLayout, []wgpu.BindGroupEntry{
		wgpu.TextureBindingEntry(0, src),
		wgpu.SamplerBindingEntry(1, r.sampler),
	})
	if err != nil {
		return fmt.Errorf("webgpu: copy bind group: %w", err)
	}
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "MyGo surface copy pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: dst, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
		}},
	})
	if err != nil {
		bind.Release()
		return fmt.Errorf("webgpu: begin copy pass: %w", err)
	}
	pass.SetPipeline(r.copyPipe)
	pass.SetBindGroup(0, bind, nil)
	pass.SetScissorRect(0, 0, uint32(max(width, 1)), uint32(max(height, 1)))
	pass.Draw(4, 1, 0, 0)
	pass.End()
	pass.Release()
	r.pending = append(r.pending, func() { bind.Release() })
	return nil
}
