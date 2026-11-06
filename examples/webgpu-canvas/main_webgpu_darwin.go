//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// WebGPU-canvas is a native MyGo window containing a custom WebGPU
// render target. The application encodes into MyGo's command encoder and
// MyGo samples the target in the same frame, without a CPU upload.
//
//	cgo-enabled macOS: go run ./examples/webgpu-canvas
//	cgo-free macOS:    CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/webgpu-canvas
package main

import (
	"log"
	"time"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const shader = `
struct Uniforms {
	size: vec4<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(
		vec2<f32>(-1.0, -1.0),
		vec2<f32>(3.0, -1.0),
		vec2<f32>(-1.0, 3.0),
	);
	return vec4<f32>(p[i], 0.0, 1.0);
}

@fragment
fn fs(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let uv = position.xy / uniforms.size.xy;
	let t = uniforms.size.z;
	let n = sin(uv.x * 18.0 + t) * cos(uv.y * 14.0 - t * 0.7);
	let d = length(uv - vec2<f32>(0.5, 0.5));
	let ring = smoothstep(0.28, 0.30, d) * (1.0 - smoothstep(0.35, 0.38, d));
	let glow = 0.55 + 0.25 * n + 0.7 * ring;
	return vec4<f32>(vec3<f32>(0.10, 0.42, 0.36) * glow, 1.0);
}
`

type app struct {
	canvas   *ui.WebGPUCanvas
	started  time.Time
	animate  bool
	pipeline *wgpu.RenderPipeline
	uniforms *wgpu.Buffer
	bind     *wgpu.BindGroup
}

func newApp() *app {
	a := &app{started: time.Now(), animate: true}
	a.canvas = ui.NewWebGPUCanvas(900, 520, a.draw)
	return a
}

func (a *app) draw(f ui.WebGPUFrame) error {
	if a.pipeline == nil {
		module, err := f.Device.CreateShaderModuleWGSL(shader)
		if err != nil {
			return err
		}
		defer module.Release()
		a.pipeline, err = f.Device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "WebGPU canvas demo",
			Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs"},
			Primitive: wgpu.PrimitiveState{
				Topology: wgpu.PrimitiveTopologyTriangleList,
			},
			Multisample: wgpu.MultisampleState{Count: 1},
			Fragment: &wgpu.FragmentState{
				Module: module, EntryPoint: "fs",
				Targets: []wgpu.ColorTargetState{{
					Format: wgpu.TextureFormatRGBA8Unorm,
				}},
			},
		})
		if err != nil {
			return err
		}
		a.uniforms, err = f.Device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "WebGPU canvas uniforms",
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
			Size:  16,
		})
		if err != nil {
			return err
		}
	}
	seconds := float32(time.Since(a.started).Seconds())
	values := [4]float32{float32(f.Width), float32(f.Height), seconds, 0}
	if err := f.Device.Queue().WriteBuffer(a.uniforms, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&values[0])), unsafe.Sizeof(values),
	)); err != nil {
		return err
	}
	view, err := f.Texture.CreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()
	pass, err := f.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "WebGPU canvas pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: 0.02, G: 0.05, B: 0.06, A: 1},
		}},
	})
	if err != nil {
		return err
	}
	defer pass.Release()
	pass.SetPipeline(a.pipeline)
	pass.SetBindGroup(0, a.bindGroup(f), nil)
	pass.Draw(3, 1, 0, 0)
	pass.End()
	return nil
}

func (a *app) bindGroup(f ui.WebGPUFrame) *wgpu.BindGroup {
	if a.bind != nil {
		return a.bind
	}
	layout := a.pipeline.GetBindGroupLayout(0)
	bind, err := f.Device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, a.uniforms, 0, 16),
	})
	if err != nil {
		panic(err)
	}
	layout.Release()
	a.bind = bind
	return bind
}

func (a *app) view(c *ui.Context) {
	a.canvas.Invalidate()
	if a.animate {
		c.AnimationFrame()
	}
	ui.Column(c).Fill().Padding(24).Gap(16).Background(ui.Hex("#0a1518")).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "MYGO / WEBGPU CANVAS").Font("monospace").FontSize(11).TextColor(ui.Hex("#82aaa0"))
			ui.Spacer(c)
			ui.Text(c, "GO-WEBGPU / NATIVE COMMAND BUFFER").Font("monospace").FontSize(10).TextColor(ui.Hex("#57716b"))
		})
		ui.Row(c).Grow(1).Gap(20).Children(func() {
			preview := ui.Box(c).Grow(1).MinWidth(0).Radius(16).Clip().Background(ui.Hex("#071113"))
			preview.Children(func() {
				ui.Image(c, a.canvas.Bitmap()).Fill()
				ui.Box(c).Absolute().Left(18).Bottom(16).Children(func() {
					ui.Text(c, "WEBGPU TEXTURE / SAME ENCODER / NO READBACK").Font("monospace").FontSize(10).TextColor(ui.Hex("#b0d0c6"))
				})
			})
			ui.Column(c).Width(250).Gap(16).Children(func() {
				ui.Text(c, "A native window\nwith a GPU guest.").Font("Baskerville").FontSize(31).TextColor(ui.Hex("#e7efe3"))
				ui.Text(c, "The fragment shader writes into MyGo's render target. The surrounding controls remain native MyGo input.").FontSize(12).LineHeight(1.5).TextColor(ui.Hex("#9bb1a8"))
				ui.Checkbox(c.Key("animate"), &a.animate, "Animate WebGPU").FontSize(12).TextColor(ui.Hex("#9bb1a8"))
				ui.Spacer(c)
				ui.Text(c, "ui.NewWebGPUCanvas\nWebGPUFrame • wgpu-native • RGBA8").Font("monospace").FontSize(10).LineHeight(1.5).TextColor(ui.Hex("#6d8980"))
			})
		})
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "MyGo WebGPU Canvas",
			Width:     1180,
			Height:    760,
			MinWidth:  900,
			MinHeight: 560,
			Content:   ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
