package webgpu

import (
	_ "embed"
	"errors"
	"fmt"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/gpu"
	"github.com/egoist/mygo/internal/scene"
)

//go:embed pass.wgsl
var passShaderSource string

const passParamsSize = uint64(unsafe.Sizeof(passParams{}))

type passParams struct {
	Origin [2]int32
	Limit  [2]int32
	Shift  [2]int32
	Dir    [2]int32
	Down   int32
	Radius int32
	Sigma  float32
	Mode   int32
}

func (r *Renderer) initPasses() error {
	shader, err := r.gpu.device.CreateShaderModuleWGSL(passShaderSource)
	if err != nil {
		return fmt.Errorf("webgpu: backdrop shader: %w", err)
	}
	r.passShader = shader
	down, err := r.passPipeline("down")
	if err != nil {
		return err
	}
	r.downPipe = down
	blur, err := r.passPipeline("blur")
	if err != nil {
		return err
	}
	r.blurPipe = blur
	layout := down.GetBindGroupLayout(0)
	if layout == nil {
		return errors.New("webgpu: backdrop pipeline has no bind group layout")
	}
	r.passLayout = layout
	r.blurLayout = blur.GetBindGroupLayout(0)
	if r.blurLayout == nil {
		return errors.New("webgpu: backdrop blur pipeline has no bind group layout")
	}
	return nil
}

func (r *Renderer) passPipeline(entry string) (*wgpu.RenderPipeline, error) {
	pipe, err := r.gpu.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "MyGo backdrop " + entry,
		Vertex: wgpu.VertexState{Module: r.passShader, EntryPoint: "pass_vs"},
		Primitive: wgpu.PrimitiveState{
			Topology: wgpu.PrimitiveTopology(5),
			CullMode: wgpu.CullModeNone,
		},
		Multisample: wgpu.MultisampleState{Count: 1},
		Fragment: &wgpu.FragmentState{
			Module: r.passShader, EntryPoint: entry,
			Targets: []wgpu.ColorTargetState{{
				Format:    wgpu.TextureFormatRGBA8Unorm,
				WriteMask: wgpu.ColorWriteMaskAll,
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("webgpu: backdrop %s pipeline: %w", entry, err)
	}
	return pipe, nil
}

func (r *Renderer) fitBackdrop(st *drawState, width, height int) error {
	if st == nil || len(st.builder.Backdrops) == 0 {
		return nil
	}
	if st.grab == nil || st.grab.w != width || st.grab.h != height || st.grab.format != st.targetFormat {
		next, err := r.createTextureWithUsage("MyGo backdrop grab",
			st.targetFormat, width, height,
			wgpu.TextureUsageTextureBinding|wgpu.TextureUsageCopyDst)
		if err != nil {
			return fmt.Errorf("webgpu: backdrop grab texture: %w", err)
		}
		if st.grab != nil {
			st.grab.Release()
		}
		st.grab = next
	}
	w, h := st.builder.BackdropSize()
	if w <= 0 || h <= 0 {
		return nil
	}
	if st.backdrop[0] != nil && st.backdrop[0].w >= w && st.backdrop[0].h >= h {
		return nil
	}
	// Leave headroom so a growing pane does not rebuild these every frame.
	w, h = max(w+w/4, 1), max(h+h/4, 1)
	for i := range st.backdrop {
		next, err := r.createRenderTexture("MyGo backdrop", wgpu.TextureFormatRGBA8Unorm, w, h)
		if err != nil {
			return fmt.Errorf("webgpu: backdrop texture: %w", err)
		}
		if st.backdrop[i] != nil {
			st.backdrop[i].Release()
		}
		st.backdrop[i] = next
	}
	return nil
}

func (r *Renderer) readBackdrop(encoder *wgpu.CommandEncoder, st *drawState, target *wgpu.Texture, bk scene.Backdrop) error {
	if target == nil || st.grab == nil || st.backdrop[0] == nil {
		return errors.New("webgpu: backdrop requires a copyable target")
	}
	encoder.CopyTextureToTexture(target, st.grab.tex, []wgpu.TextureCopy{{
		Source:      wgpu.ImageCopyTexture{Texture: target, Aspect: wgpu.TextureAspectAll},
		Destination: wgpu.ImageCopyTexture{Texture: st.grab.tex, Aspect: wgpu.TextureAspectAll},
		Size: wgpu.Extent3D{
			Width: uint32(st.sceneWidth), Height: uint32(st.sceneHeight), DepthOrArrayLayers: 1,
		},
	}})
	w, h := bk.Size()
	p := gpu.DownPass(bk, [2]int32{})
	if err := r.encodePass(encoder, r.downPipe, r.passLayout, st.grab.view, st.backdrop[0].view, w, h, p); err != nil {
		return err
	}
	if bk.Radius == 0 {
		return nil
	}
	p = gpu.BlurPass(bk, [2]int32{1, 0})
	if err := r.encodePass(encoder, r.blurPipe, r.blurLayout, st.backdrop[0].view, st.backdrop[1].view, w, h, p); err != nil {
		return err
	}
	p = gpu.BlurPass(bk, [2]int32{0, 1})
	return r.encodePass(encoder, r.blurPipe, r.blurLayout, st.backdrop[1].view, st.backdrop[0].view, w, h, p)
}

func (r *Renderer) encodePass(
	encoder *wgpu.CommandEncoder,
	pipe *wgpu.RenderPipeline,
	layout *wgpu.BindGroupLayout,
	src *wgpu.TextureView,
	dst *wgpu.TextureView,
	width, height int,
	p gpu.Pass,
) error {
	params := passParams{
		Origin: p.Origin, Limit: p.Limit, Shift: p.Shift, Dir: p.Dir,
		Down: p.Down, Radius: p.Radius, Sigma: p.Sigma, Mode: p.Mode,
	}
	buf, err := r.gpu.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "MyGo backdrop params",
		Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size:  passParamsSize,
	})
	if err != nil {
		return fmt.Errorf("webgpu: backdrop params buffer: %w", err)
	}
	if err := r.gpu.queue.WriteBuffer(buf, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&params)), int(passParamsSize),
	)); err != nil {
		buf.Release()
		return fmt.Errorf("webgpu: write backdrop params: %w", err)
	}
	bind, err := r.gpu.device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, buf, 0, passParamsSize),
		wgpu.TextureBindingEntry(1, src),
	})
	if err != nil {
		buf.Release()
		return fmt.Errorf("webgpu: backdrop bind group: %w", err)
	}
	r.pending = append(r.pending, func() {
		bind.Release()
		buf.Release()
	})
	pass, err := encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "MyGo backdrop pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: dst, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{},
		}},
	})
	if err != nil {
		return fmt.Errorf("webgpu: begin backdrop pass: %w", err)
	}
	pass.SetPipeline(pipe)
	pass.SetBindGroup(0, bind, nil)
	pass.SetScissorRect(0, 0, uint32(max(width, 1)), uint32(max(height, 1)))
	pass.Draw(4, 1, 0, 0)
	pass.End()
	pass.Release()
	return nil
}
