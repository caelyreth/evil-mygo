//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// WebGPU-layer presents an independent native UILayer through a WebGPU
// texture. The left viewport is a custom pass; the right side is the
// host window's ordinary native UI.
//
//	go run ./examples/webgpu-layer
package main

import (
	"errors"
	"log"
	"math"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/caelyreth/evil-mygo"
	"github.com/caelyreth/evil-mygo/ui"
)

const (
	layerWidth  = 900
	layerHeight = 600
)

const layerShader = `
@group(0) @binding(0) var src: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;

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
	let size = vec2<f32>(textureDimensions(src));
	let c = textureSampleLevel(src, samp, position.xy / size, 0.0);
	let edge = smoothstep(0.0, 0.55, min(position.x, position.y));
	return vec4<f32>(mix(vec3<f32>(0.08, 0.10, 0.11), c.rgb, edge), 1.0);
}
`

type app struct {
	canvas  *ui.WebGPUCanvas
	layer   *ui.UILayer
	surface *ui.PlanarSurface
	host    ui.Handle
	ctx     *ui.Context
	name    string
	clicks  int

	pipeline   *wgpu.RenderPipeline
	sampler    *wgpu.Sampler
	bind       *wgpu.BindGroup
	sourceView *wgpu.TextureView
	sourceAt   *wgpu.Texture
}

func newApp() *app {
	a := &app{name: "A native layer"}
	a.surface = ui.NewPlanarSurface(ui.Rect{W: layerWidth, H: layerHeight})
	a.layer = ui.NewUILayer(a.layerView, layerWidth, layerHeight)
	a.canvas = ui.NewWebGPUCanvas(layerWidth, layerHeight, a.draw)
	a.canvas.SetSource(a.layer.Bitmap())
	return a
}

func (a *app) draw(f ui.WebGPUFrame) error {
	if f.Source == nil || f.SourceTexture == nil {
		return nil
	}
	if a.pipeline == nil {
		module, err := f.Device.CreateShaderModuleWGSL(layerShader)
		if err != nil {
			return err
		}
		a.pipeline, err = f.Device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "WebGPU layer pass",
			Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs"},
			Primitive: wgpu.PrimitiveState{
				Topology: wgpu.PrimitiveTopologyTriangleList,
				CullMode: wgpu.CullModeNone,
			},
			Multisample: wgpu.MultisampleState{Count: 1},
			Fragment: &wgpu.FragmentState{
				Module: module, EntryPoint: "fs",
				Targets: []wgpu.ColorTargetState{{
					Format: wgpu.TextureFormatRGBA8Unorm, WriteMask: wgpu.ColorWriteMaskAll,
				}},
			},
		})
		module.Release()
		if err != nil {
			return err
		}
		a.sampler, err = f.Device.CreateLinearSampler()
		if err != nil {
			return err
		}
	}
	bind, err := a.bindGroup(f)
	if err != nil {
		return err
	}
	target, err := f.Texture.CreateView(nil)
	if err != nil {
		return err
	}
	defer target.Release()
	pass, err := f.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "WebGPU layer presentation",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: target, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: 0.02, G: 0.03, B: 0.035, A: 1},
		}},
	})
	if err != nil {
		return err
	}
	pass.SetPipeline(a.pipeline)
	pass.SetBindGroup(0, bind, nil)
	pass.Draw(3, 1, 0, 0)
	pass.End()
	pass.Release()
	return nil
}

func (a *app) bindGroup(f ui.WebGPUFrame) (*wgpu.BindGroup, error) {
	if a.bind != nil && a.sourceAt == f.SourceTexture {
		return a.bind, nil
	}
	if a.bind != nil {
		a.bind.Release()
		a.bind = nil
	}
	if a.sourceView != nil {
		a.sourceView.Release()
		a.sourceView = nil
	}
	view, err := f.SourceTexture.CreateView(nil)
	if err != nil {
		return nil, err
	}
	layout := a.pipeline.GetBindGroupLayout(0)
	if layout == nil {
		view.Release()
		return nil, errors.New("WebGPU layer has no bind group layout")
	}
	bind, err := f.Device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
		wgpu.TextureBindingEntry(0, view),
		wgpu.SamplerBindingEntry(1, a.sampler),
	})
	layout.Release()
	if err != nil {
		view.Release()
		return nil, err
	}
	a.bind, a.sourceView, a.sourceAt = bind, view, f.SourceTexture
	return bind, nil
}

func (a *app) view(c *ui.Context) {
	a.ctx = c
	a.layer.SetScale(c.PixelsPerDIP())
	if a.layer.NeedsFrame() {
		a.layer.Render()
		a.canvas.Invalidate()
	}
	ui.Row(c).Fill().Background(ui.Hex("#111416")).Children(func() {
		viewport := ui.Box(c).Grow(1).MinWidth(0).FillHeight().Bind(&a.host).
			Label("WebGPU native layer viewport")
		if box := viewport.LayoutBounds(); box.W > 0 && box.H > 0 {
			scale := c.PixelsPerDIP()
			a.canvas.Resize(
				int(math.Ceil(float64(box.W*scale))),
				int(math.Ceil(float64(box.H*scale))),
			)
		}
		viewport.HostLayer(a.layer, a.surface)
		viewport.HandleInput(func(ev ui.InputEvent) bool {
			handled := a.host.HandleLayerInput(c, ev)
			if handled {
				a.layer.Render()
				a.canvas.Invalidate()
				c.Invalidate()
			}
			return handled
		})
		viewport.Children(func() {
			ui.Image(c, a.canvas.Bitmap()).Fill()
		})
		ui.Column(c).Width(300).Shrink(0).FillHeight().Padding(28).Gap(16).
			Background(ui.Hex("#e8e7e2")).Children(func() {
			ui.Text(c, "MYGO / WEBGPU LAYER").Font("monospace").FontSize(10).TextColor(ui.Hex("#666660"))
			ui.Text(c, "A layer,\nnot a screenshot.").Font("Baskerville").FontSize(36).
				LineHeight(.92).TextColor(ui.Hex("#171715"))
			ui.Text(c, "The left side is an independent native UILayer sampled by a WebGPU texture.").
				FontSize(12).LineHeight(1.45).TextColor(ui.Hex("#666660"))
			ui.Box(c).Height(1).Background(ui.Hex("#c9c7c0"))
			ui.Column(c).Padding(14).Radius(10).Background(ui.Hex("#dcdad3")).Gap(3).Children(func() {
				ui.Text(c, "SIGNALS").Font("monospace").FontSize(10).TextColor(ui.Hex("#666660"))
				ui.Textf(c, "%02d", a.clicks).Font("Baskerville").FontSize(48).TextColor(ui.Hex("#171715"))
			})
			ui.Text(c, "Click the native controls inside the GPU surface. Pointer input is projected through SurfaceMap before it reaches the layer.").
				FontSize(12).LineHeight(1.45).TextColor(ui.Hex("#666660"))
			ui.Spacer(c)
			ui.Text(c, "ui.NewUILayer\nWebGPUCanvas.SetSource\nPlanarSurface / HostLayer").
				Font("monospace").FontSize(10).LineHeight(1.6).TextColor(ui.Hex("#555550"))
		})
	})
}

func (a *app) layerView(c *ui.Context) {
	ui.Column(c).Fill().Padding(48).Gap(18).Background(ui.Hex("#ecebe5")).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "NATIVE LAYER / 01").Font("monospace").FontSize(11).TextColor(ui.Hex("#60605a"))
			ui.Spacer(c)
			ui.Box(c).Size(10, 10).Radius(5).Background(ui.Hex("#222220"))
		})
		ui.Text(c, "A live\nnative UI.").Font("Baskerville").FontSize(72).LineHeight(.88).TextColor(ui.Hex("#171715"))
		ui.Text(c, "This form is laid out, painted and edited by MyGo off-window.").FontSize(15).
			LineHeight(1.45).TextColor(ui.Hex("#5d5d57"))
		ui.TextInput(c.Key("layer-name"), &a.name).Label("Layer name").Width(360).Height(38)
		ui.Spacer(c)
		button := ui.ButtonBase(c.Key("press-layer")).Width(320).Height(58).Radius(10).
			Background(ui.Hex("#202020")).Label("Press layer").Cursor(ui.CursorPointer)
		button.Children(func() {
			ui.Text(c, "PRESS LAYER").Font("monospace").FontSize(14).Bold().TextColor(ui.Hex("#f4f3ee"))
		})
		if button.Clicked() {
			a.clicks++
			a.canvas.Invalidate()
			c.Invalidate()
		}
		ui.Textf(c, "SIGNALS / %02d", a.clicks).Font("monospace").FontSize(11).TextColor(ui.Hex("#60605a"))
	})
}

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:          "MyGo WebGPU Layer",
			Width:          1240,
			Height:         760,
			MinWidth:       1000,
			MinHeight:      680,
			TitleBarStyle:  mygo.TitleBarHiddenInset,
			TitleBarHeight: 48,
			Content:        ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
