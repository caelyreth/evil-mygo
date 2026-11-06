package webgpu

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/scene"
)

//go:embed effect.wgsl
var effectShaderSource string

const effectParamsSize = uint64(17 * 16)

type effectPipeline struct {
	pipe   *wgpu.RenderPipeline
	layout *wgpu.BindGroupLayout
}

func (r *Renderer) effectPipelineFor(fx *scene.Effect) (*effectPipeline, error) {
	if fx == nil {
		return nil, errors.New("webgpu: nil effect")
	}
	if fx.WGSL == "" {
		return nil, errors.New("webgpu: effect has no WGSL source")
	}
	if p := r.effects[fx.WGSL]; p != nil {
		return p, nil
	}
	head, tail, ok := strings.Cut(effectShaderSource, "\n// effect\n")
	if !ok {
		return nil, errors.New("webgpu: effect shader template is malformed")
	}
	source := head + "\n" + fx.WGSL + "\n" + tail
	shader, err := r.gpu.device.CreateShaderModuleWGSL(source)
	if err != nil {
		return nil, fmt.Errorf("webgpu: effect shader %s: %w", fx.Name, err)
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
	pipe, err := r.gpu.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "MyGo effect " + fx.Name,
		Vertex: wgpu.VertexState{Module: shader, EntryPoint: "effect_vs"},
		Primitive: wgpu.PrimitiveState{
			Topology: wgpu.PrimitiveTopology(5),
			CullMode: wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1},
		Fragment: &wgpu.FragmentState{
			Module: shader, EntryPoint: "effect_fs",
			Targets: []wgpu.ColorTargetState{{
				Format: r.format, Blend: blend, WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	shader.Release()
	if err != nil {
		return nil, fmt.Errorf("webgpu: effect pipeline %s: %w", fx.Name, err)
	}
	layout := pipe.GetBindGroupLayout(0)
	if layout == nil {
		pipe.Release()
		return nil, errors.New("webgpu: effect pipeline has no bind group layout")
	}
	p := &effectPipeline{pipe: pipe, layout: layout}
	r.effects[fx.WGSL] = p
	return p, nil
}

func (r *Renderer) drawEffect(pass *wgpu.RenderPassEncoder, st *drawState, batch gpu.Batch) error {
	if batch.Effect == nil {
		return nil
	}
	pipeline, err := r.effectPipelineFor(batch.Effect)
	if err != nil {
		return err
	}
	if batch.Start < 0 || batch.Start >= len(st.builder.Instances) {
		return errors.New("webgpu: effect batch has no instance")
	}
	in := st.builder.Instances[batch.Start]
	var params [17][4]float32
	params[0] = [4]float32{float32(st.sceneWidth), float32(st.sceneHeight), 0, 0}
	params[1] = in.Rect
	params[2] = absRadii(in.Radii)
	params[3], params[4], params[5], params[6], params[7] = in.Inner, in.Color, in.Color2, in.Border, in.Grad
	params[8] = in.UV
	params[9], params[10] = in.Clip, in.ClipRadii
	params[11], params[12], params[13] = in.Transform0, in.Transform1, in.Transform2
	params[14], params[15], params[16] = in.Params, in.Inner, in.UV
	uniform, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo effect params",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size:  effectParamsSize,
	})
	if err != nil {
		return fmt.Errorf("webgpu: effect params buffer: %w", err)
	}
	if err := r.gpu.queue.WriteBuffer(uniform, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&params[0])), int(effectParamsSize),
	)); err != nil {
		uniform.Release()
		return fmt.Errorf("webgpu: write effect params: %w", err)
	}
	shaderUniforms, shaderUniformBytes, err := r.effectUniformBuffer(batch)
	if err != nil {
		uniform.Release()
		return err
	}
	textures := [4]*wgpu.TextureView{
		r.defaultTransparent.view, r.defaultTransparent.view,
		r.defaultTransparent.view, r.defaultTransparent.view,
	}
	for i, token := range batch.Textures {
		if token == 0 {
			continue
		}
		if t := r.texture(token); t != nil {
			textures[i] = t.view
		}
	}
	clip := r.defaultClip.view
	if batch.ClipMask != 0 {
		if t := r.texture(batch.ClipMask); t != nil {
			clip = t.view
		}
	}
	bind, err := r.gpu.device.CreateBindGroupSimple(pipeline.layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, uniform, 0, effectParamsSize),
		wgpu.BufferBindingEntry(1, st.instBuf, 0, uint64(st.capacity)*uint64(instanceSize)),
		wgpu.TextureBindingEntry(2, textures[0]),
		wgpu.TextureBindingEntry(3, textures[1]),
		wgpu.TextureBindingEntry(4, textures[2]),
		wgpu.TextureBindingEntry(5, textures[3]),
		wgpu.SamplerBindingEntry(6, r.sampler),
		wgpu.TextureBindingEntry(7, clip),
		wgpu.TextureBindingEntry(8, st.mask.view),
		wgpu.BufferBindingEntry(9, shaderUniforms, 0, shaderUniformBytes),
	})
	if err != nil {
		uniform.Release()
		shaderUniforms.Release()
		return fmt.Errorf("webgpu: effect bind group: %w", err)
	}
	r.pending = append(r.pending, func() {
		bind.Release()
		uniform.Release()
		shaderUniforms.Release()
	})
	pass.SetPipeline(pipeline.pipe)
	pass.SetBindGroup(0, bind, nil)
	pass.SetScissorRect(
		uint32(max(batch.Scissor.Left, 0)),
		uint32(max(batch.Scissor.Top, 0)),
		uint32(max(batch.Scissor.Right-batch.Scissor.Left, 0)),
		uint32(max(batch.Scissor.Bottom-batch.Scissor.Top, 0)),
	)
	pass.Draw(4, uint32(batch.Count), 0, uint32(batch.Start))
	return nil
}

func (r *Renderer) effectUniformBuffer(batch gpu.Batch) (*wgpu.Buffer, uint64, error) {
	packed := make([]byte, 16)
	if batch.Shader != nil && batch.Effect.Shader != nil {
		packed = batch.Effect.Shader.Pack(nil, batch.Uniforms)
		if len(packed) < 16 {
			packed = append(packed, make([]byte, 16-len(packed))...)
		} else if rem := len(packed) % 16; rem != 0 {
			packed = append(packed, make([]byte, 16-rem)...)
		}
	}
	buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo effect uniforms",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size:  uint64(len(packed)),
	})
	if err != nil {
		return nil, 0, fmt.Errorf("webgpu: effect uniforms buffer: %w", err)
	}
	if err := r.gpu.queue.WriteBuffer(buf, 0, packed); err != nil {
		buf.Release()
		return nil, 0, fmt.Errorf("webgpu: write effect uniforms: %w", err)
	}
	return buf, uint64(len(packed)), nil
}

func absRadii(r [4]float32) [4]float32 {
	for i := range r {
		if r[i] < 0 {
			r[i] = -r[i]
		}
	}
	return r
}
