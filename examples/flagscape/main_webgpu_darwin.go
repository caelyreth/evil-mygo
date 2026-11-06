//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

// Flagscape is a native MyGo window whose left side is a WebGPU 3D scene
// and whose right side is native MyGo UI. The scene samples a captured
// native face; the face is not a screenshot uploaded from the CPU.
//
//	go run ./examples/flagscape
//
// WASD moves the camera, Q/E changes height, and dragging the viewport
// looks around. The Activate control is hit-tested through the panel's
// camera ray and UVs.
package main

import (
	"errors"
	"log"
	"math"
	"time"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const shader = `
struct Uniforms {
	viewport: vec4<f32>,
	camera: vec4<f32>,
	look: vec4<f32>,
}

@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var src: texture_2d<f32>;
@group(0) @binding(2) var samp: sampler;

@vertex
fn vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(
		vec2<f32>(-1.0, -1.0),
		vec2<f32>(3.0, -1.0),
		vec2<f32>(-1.0, 3.0),
	);
	return vec4<f32>(p[i], 0.0, 1.0);
}

fn planeT(cam: vec3<f32>, rd: vec3<f32>, z: f32) -> f32 {
	if (abs(rd.z) <= 0.02) {
		return -1.0;
	}
	return (z - cam.z) / rd.z;
}

fn panelGrid(p: vec3<f32>) -> vec3<f32> {
	let g = abs(fract(p.xz * 4.0) - vec2<f32>(0.5));
	let line = 1.0 - smoothstep(0.0, 0.025, min(g.x, g.y));
	let falloff = 1.0 - smoothstep(0.0, 3.5, length(p.xz));
	var c = mix(vec3<f32>(0.43), vec3<f32>(0.73), falloff);
	c += vec3<f32>(0.12) * line * exp(-abs(p.x) * 0.6);
	return c;
}

@fragment
fn fs(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let uv = position.xy / u.viewport.xy;
	var ndc = uv * 2.0 - vec2<f32>(1.0);
	ndc.y = -ndc.y;

	let cam = u.camera.xyz;
	let yaw = u.camera.w;
	let pitch = u.look.x;
	let fwd = normalize(vec3<f32>(sin(yaw) * cos(pitch), sin(pitch), -cos(yaw) * cos(pitch)));
	let right = normalize(cross(fwd, vec3<f32>(0.0, 1.0, 0.0)));
	let up = normalize(cross(right, fwd));
	let aspect = u.viewport.x / max(u.viewport.y, 1.0);
	let rd = normalize(fwd + right * ndc.x * 0.74 * aspect + up * ndc.y * 0.74);

	var col = mix(vec3<f32>(0.70), vec3<f32>(0.95), smoothstep(-0.22, 0.68, rd.y));
	let sun = pow(max(dot(rd, normalize(vec3<f32>(0.42, 0.34, 0.84))), 0.0), 80.0);
	col += vec3<f32>(0.12) * sun;

	var best = 1e9;
	let groundY = -0.72;
	if (rd.y < -0.001) {
		let tg = (groundY - cam.y) / rd.y;
		if (tg > 0.0 && tg < best) {
			let p = cam + rd * tg;
			best = tg;
			col = panelGrid(p);
			col *= 1.0 - exp(-pow(length(p.xz) / 1.25, 2.0)) * 0.10;
		}
	}

	let panelY = 0.28;
	let thickness = 0.025;
	let halfH = 0.68;
	let sourceAspect = u.viewport.z / max(u.viewport.w, 1.0);
	let halfW = halfH * sourceAspect;
	let tf = planeT(cam, rd, thickness);
	let tb = planeT(cam, rd, -thickness);
	var tp = 1e9;
	var back = false;
	if (tf > 0.01) {
		tp = tf;
	}
	if (tb > 0.01 && tb < tp) {
		tp = tb;
		back = true;
	}

	var ts = 1e9;
	var sideNormal = vec3<f32>(0.0);
	if (abs(rd.x) > 0.001) {
		let x = select(-halfW, halfW, rd.x > 0.0);
		let t = (x - cam.x) / rd.x;
		let p = cam + rd * t;
		if (t > 0.01 && t < ts && abs(p.y - panelY) <= halfH && abs(p.z) <= thickness) {
			ts = t;
			sideNormal = vec3<f32>(sign(x), 0.0, 0.0);
		}
	}
	if (abs(rd.y) > 0.001) {
		let y = select(panelY - halfH, panelY + halfH, rd.y > 0.0);
		let t = (y - cam.y) / rd.y;
		let p = cam + rd * t;
		if (t > 0.01 && t < ts && abs(p.x) <= halfW && abs(p.z) <= thickness) {
			ts = t;
			sideNormal = vec3<f32>(0.0, sign(y - panelY), 0.0);
		}
	}
	let isSide = ts < tp;
	if (isSide) {
		tp = ts;
		back = false;
	}
	if (tp > 0.01 && tp < best) {
		let p = cam + rd * tp;
		if (isSide) {
			let light = 0.28 + 0.26 * max(dot(sideNormal, normalize(vec3<f32>(-0.55, 0.70, 0.45))), 0.0);
			col = mix(col, vec3<f32>(light), smoothstep(0.0, 0.001, thickness - abs(p.z)));
		} else {
			var fuv = vec2<f32>((p.x + halfW) / (2.0 * halfW), (halfH - (p.y - panelY)) / (2.0 * halfH));
			if (back) {
				fuv.x = 1.0 - fuv.x;
			}
			let tex = textureSample(src, samp, clamp(fuv, vec2<f32>(0.0), vec2<f32>(1.0)));
			let n = select(vec3<f32>(0.0, 0.0, 1.0), vec3<f32>(0.0, 0.0, -1.0), back);
			let L = normalize(vec3<f32>(-0.55, 0.70, 0.45));
			let V = -rd;
			let diffuse = max(dot(n, L), 0.0);
			let spec = pow(max(dot(n, normalize(L + V)), 0.0), 36.0) * select(0.025, 0.08, back);
			let edgeX = 1.0 - smoothstep(halfW - 0.0015, halfW + 0.0015, abs(p.x));
			let edgeY = 1.0 - smoothstep(halfH - 0.0015, halfH + 0.0015, abs(p.y - panelY));
			let albedo = dot(tex.rgb, vec3<f32>(0.299, 0.587, 0.114));
			let shade = albedo * (0.48 + 0.42 * diffuse * select(1.0, 0.65, back)) + spec;
			col = mix(col, vec3<f32>(shade), tex.a * edgeX * edgeY);
		}
	}

	let q = uv - vec2<f32>(0.5);
	col *= 1.0 - dot(q, q) * 0.28;
	let gray = dot(col, vec3<f32>(0.299, 0.587, 0.114));
	return vec4<f32>(vec3<f32>(pow(max(gray, 0.0), 0.88)), 1.0);
}
`

type camera struct {
	x, y, z float32
	yaw     float32
	pitch   float32
}

type app struct {
	canvas *ui.WebGPUCanvas
	cam    camera

	last  time.Time
	drag  bool
	lastX float32
	lastY float32
	acts  int

	viewBox ui.Rect
	hitBox  ui.Rect

	pipeline *wgpu.RenderPipeline
	uniform  *wgpu.Buffer
	sampler  *wgpu.Sampler
	bind     *wgpu.BindGroup
	source   *wgpu.TextureView
	sourceAt *wgpu.Texture
}

func newApp() *app {
	a := &app{cam: camera{z: 3.8, y: 0.16}}
	a.canvas = ui.NewWebGPUCanvas(1600, 1000, a.draw)
	return a
}

func (a *app) draw(f ui.WebGPUFrame) error {
	if f.Source == nil || f.SourceTexture == nil {
		return nil
	}
	if a.pipeline == nil {
		module, err := f.Device.CreateShaderModuleWGSL(shader)
		if err != nil {
			return err
		}
		a.pipeline, err = f.Device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "Flagscape panel",
			Vertex: wgpu.VertexState{Module: module, EntryPoint: "vs"},
			Primitive: wgpu.PrimitiveState{
				Topology: wgpu.PrimitiveTopologyTriangleList,
				CullMode: wgpu.CullModeNone,
			},
			Multisample: wgpu.MultisampleState{Count: 1},
			Fragment: &wgpu.FragmentState{
				Module: module, EntryPoint: "fs",
				Targets: []wgpu.ColorTargetState{{
					Format: wgpu.TextureFormatRGBA8Unorm,
				}},
			},
		})
		module.Release()
		if err != nil {
			return err
		}
		a.uniform, err = f.Device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "Flagscape uniforms",
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
			Size:  48,
		})
		if err != nil {
			return err
		}
		a.sampler, err = f.Device.CreateLinearSampler()
		if err != nil {
			return err
		}
	}

	values := [12]float32{
		float32(f.Width), float32(f.Height), float32(f.SourceWidth), float32(f.SourceHeight),
		a.cam.x, a.cam.y, a.cam.z, a.cam.yaw,
		a.cam.pitch, 0, 0, 0,
	}
	if err := f.Device.Queue().WriteBuffer(a.uniform, 0, unsafe.Slice(
		(*byte)(unsafe.Pointer(&values[0])), unsafe.Sizeof(values),
	)); err != nil {
		return err
	}
	bind, err := a.bindGroup(f)
	if err != nil {
		return err
	}
	view, err := f.Texture.CreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()
	pass, err := f.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "Flagscape panel pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{},
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
	if a.source != nil {
		a.source.Release()
		a.source = nil
	}
	view, err := f.SourceTexture.CreateView(nil)
	if err != nil {
		return nil, err
	}
	layout := a.pipeline.GetBindGroupLayout(0)
	if layout == nil {
		view.Release()
		return nil, errors.New("Flagscape pipeline has no bind group layout")
	}
	bind, err := f.Device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, a.uniform, 0, 48),
		wgpu.TextureBindingEntry(1, view),
		wgpu.SamplerBindingEntry(2, a.sampler),
	})
	layout.Release()
	if err != nil {
		view.Release()
		return nil, err
	}
	a.bind, a.source, a.sourceAt = bind, view, f.SourceTexture
	return bind, nil
}

func (a *app) view(c *ui.Context) {
	a.tick(c)
	ui.Row(c).Fill().Background(ui.Hex("#111111")).Children(func() {
		a.viewport(c)
		a.sidebar(c)
	})
}

func (a *app) tick(c *ui.Context) {
	now := c.Now()
	if a.last.IsZero() {
		a.last = now
	}
	dt := min(now.Sub(a.last).Seconds(), 0.05)
	a.last = now
	moving := false
	fwd := float32(math.Sin(float64(a.cam.yaw)))
	back := float32(math.Cos(float64(a.cam.yaw)))
	rightX := float32(math.Cos(float64(a.cam.yaw)))
	rightZ := float32(math.Sin(float64(a.cam.yaw)))
	const speed = float32(1.7)
	if c.KeyDown(ui.KeyW) {
		a.cam.x += fwd * speed * float32(dt)
		a.cam.z -= back * speed * float32(dt)
		moving = true
	}
	if c.KeyDown(ui.KeyS) {
		a.cam.x -= fwd * speed * float32(dt)
		a.cam.z += back * speed * float32(dt)
		moving = true
	}
	if c.KeyDown(ui.KeyA) {
		a.cam.x -= rightX * speed * float32(dt)
		a.cam.z -= rightZ * speed * float32(dt)
		moving = true
	}
	if c.KeyDown(ui.KeyD) {
		a.cam.x += rightX * speed * float32(dt)
		a.cam.z += rightZ * speed * float32(dt)
		moving = true
	}
	if c.KeyDown(ui.KeyQ) {
		a.cam.y -= speed * float32(dt)
		moving = true
	}
	if c.KeyDown(ui.KeyE) {
		a.cam.y += speed * float32(dt)
		moving = true
	}
	if moving || a.drag {
		a.canvas.Invalidate()
		c.AnimationFrame()
	}
}

func (a *app) viewport(c *ui.Context) {
	view := ui.Box(c).Grow(1).MinWidth(0).FillHeight().Focusable().
		Label("Interactive WebGPU panel viewport")
	view.Cache().Effect(a.canvas.Effect())
	a.viewBox = view.LayoutBounds()
	if a.viewBox.W > 0 && a.viewBox.H > 0 {
		scale := c.PixelsPerDIP()
		a.canvas.Resize(
			int(math.Ceil(float64(a.viewBox.W*scale))),
			int(math.Ceil(float64(a.viewBox.H*scale))),
		)
	}
	if a.drag {
		view.Cursor(ui.CursorNone)
	} else if x, y, over := view.PointerPosition(); over && a.hitAction(x, y) {
		view.Cursor(ui.CursorPointer)
	} else {
		view.Cursor(ui.CursorCrosshair)
	}
	view.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			if a.hitAction(ev.X, ev.Y) {
				a.acts++
				a.canvas.Invalidate()
				return true
			}
			a.drag = true
			a.lastX, a.lastY = ev.X, ev.Y
			a.canvas.Invalidate()
			return true
		case ui.InputPointerMove:
			if !a.drag {
				return false
			}
			a.cam.yaw += (ev.X - a.lastX) * 0.006
			a.cam.pitch = clamp(a.cam.pitch-(ev.Y-a.lastY)*0.006, -1.05, 1.05)
			a.lastX, a.lastY = ev.X, ev.Y
			a.canvas.Invalidate()
			return true
		case ui.InputPointerUp:
			if ev.Button == 0 && a.drag {
				a.drag = false
				a.canvas.Invalidate()
				return true
			}
		case ui.InputScroll:
			a.cam.z = max(1.3, a.cam.z-ev.DY*0.006)
			a.canvas.Invalidate()
			return true
		}
		return false
	})
	view.Children(func() { a.face(c) })
}

func (a *app) face(c *ui.Context) {
	surface := ui.Box(c).Fill().IgnoreInput(true)
	surface.Draw(func(p *ui.Painter, r ui.Rect) {
		p.FillGradient(r, ui.LinearGradient{
			From: ui.Hex("#f0f0f0"), To: ui.Hex("#b8b8b8"), Angle: 135,
			Stops: []ui.GradientStop{
				{Color: ui.Hex("#f5f5f5"), At: 0},
				{Color: ui.Hex("#d6d6d6"), At: .58},
				{Color: ui.Hex("#aaaaaa"), At: 1},
			},
		}, 0)
	})
	surface.Children(func() {
		ui.Column(c).Fill().Padding(56, 48).Gap(22).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "MYGO / WEBGPU SURFACE").Font("monospace").FontSize(12).TextColor(ui.Hex("#3d3d3d"))
				ui.Spacer(c)
				ui.Box(c).Size(11, 11).Radius(6).Background(ui.Hex("#303030"))
			})
			ui.Column(c).Gap(9).Children(func() {
				ui.Text(c, "A SURFACE\nIN SPACE.").Font("Baskerville").FontSize(86).LineHeight(.88).TextColor(ui.Hex("#171717"))
				ui.Text(c, "A native MyGo interface, sampled by WebGPU in a 3D scene.").FontSize(16).LineHeight(1.45).TextColor(ui.Hex("#4b4b4b"))
			})
			ui.Spacer(c)
			action := ui.ButtonBase(c.Key("activate")).Width(360).Height(72).AlignSelf(ui.Center).
				Radius(12).Background(ui.Hex("#1b1b1b")).Border(0, ui.Transparent).
				Label("Activate").IgnoreInput(true)
			action.Children(func() {
				ui.Text(c, "ACTIVATE").Font("monospace").FontSize(17).Bold().TextColor(ui.Hex("#ffffff"))
			})
			a.hitBox = action.LayoutBounds()
			ui.Row(c).Children(func() {
				ui.Textf(c, "ACTIVATIONS  /  %02d", a.acts).Font("monospace").FontSize(11).TextColor(ui.Hex("#3d3d3d"))
				ui.Spacer(c)
				ui.Text(c, "NATIVE UI / GRAYSCALE").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			})
		})
	})
}

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).Width(300).Shrink(0).FillHeight().Padding(26).Gap(16).
		Background(ui.Hex("#eeeeee")).Children(func() {
		ui.Text(c, "FLAGSCAPE / 03").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
		ui.Text(c, "The scene is\nan input.").Font("Baskerville").FontSize(34).LineHeight(.92).TextColor(ui.Hex("#171717"))
		ui.Box(c).Height(1).Background(ui.Hex("#c7c7c7"))
		ui.Column(c).Padding(14).Radius(10).Background(ui.Hex("#dddddd")).Gap(3).Children(func() {
			ui.Text(c, "ACTIVATIONS").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			ui.Textf(c, "%02d", a.acts).Font("Baskerville").FontSize(48).TextColor(ui.Hex("#171717"))
		})
		ui.Column(c).Gap(4).Children(func() {
			ui.Text(c, "CAMERA").Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
			ui.Textf(c, "X %+.2f  Y %+.2f  Z %+.2f", a.cam.x, a.cam.y, a.cam.z).
				Font("monospace").FontSize(10).TextColor(ui.Hex("#313131"))
			ui.Textf(c, "YAW %+.1f  PITCH %+.1f", a.cam.yaw*180/math.Pi, a.cam.pitch*180/math.Pi).
				Font("monospace").FontSize(10).TextColor(ui.Hex("#555555"))
		})
		ui.Spacer(c)
		reset := ui.ButtonBase(c.Key("reset")).FillWidth().Height(42).Radius(9).
			Background(ui.Hex("#1b1b1b")).Cursor(ui.CursorPointer).Label("Reset view")
		reset.Children(func() {
			ui.Text(c, "RESET VIEW").Font("monospace").FontSize(11).Bold().TextColor(ui.Hex("#ffffff"))
		})
		if reset.Clicked() {
			a.cam = camera{z: 3.8, y: 0.16}
			a.canvas.Invalidate()
		}
		ui.Text(c, "WASD  walk     Q/E  height\nDRAG  mouse-look     WHEEL  zoom").
			Font("monospace").FontSize(10).LineHeight(1.5).TextColor(ui.Hex("#555555"))
		ui.Text(c, "The left viewport is a WebGPU scene. Its face is a captured native MyGo subtree; pointer rays become panel UVs for input.").
			FontSize(11).LineHeight(1.45).TextColor(ui.Hex("#666666"))
	})
}

func (a *app) hitAction(x, y float32) bool {
	if a.viewBox.W <= 0 || a.viewBox.H <= 0 || a.hitBox.W <= 0 {
		return false
	}
	u, v, ok := a.panelUV(x, y, a.viewBox.W, a.viewBox.H)
	if !ok {
		return false
	}
	px := a.viewBox.X + u*a.viewBox.W
	py := a.viewBox.Y + v*a.viewBox.H
	return px >= a.hitBox.X && px < a.hitBox.X+a.hitBox.W &&
		py >= a.hitBox.Y && py < a.hitBox.Y+a.hitBox.H
}

func (a *app) panelUV(x, y, w, h float32) (float32, float32, bool) {
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}
	aspect := w / h
	halfH := float32(0.68)
	halfW := halfH * aspect
	ndcX := x/w*2 - 1
	ndcY := 1 - y/h*2
	fwd := vec3{
		float32(math.Sin(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
		float32(math.Sin(float64(a.cam.pitch))),
		-float32(math.Cos(float64(a.cam.yaw))) * float32(math.Cos(float64(a.cam.pitch))),
	}
	right := normalize3(cross3(fwd, vec3{0, 1, 0}))
	up := normalize3(cross3(right, fwd))
	rd := normalize3(add3(fwd, add3(scale3(right, ndcX*0.74*aspect), scale3(up, ndcY*0.74))))
	if abs32(rd.z) <= 0.02 {
		return 0, 0, false
	}
	cam := vec3{a.cam.x, a.cam.y, a.cam.z}
	front := a.intersectPanel(cam, rd, halfW, halfH, 0.025)
	back := a.intersectPanel(cam, rd, halfW, halfH, -0.025)
	t := float32(-1)
	if front > 0 && (back <= 0 || front < back) {
		t = front
	} else if back > 0 {
		t = back
	}
	if t <= 0.01 {
		return 0, 0, false
	}
	p := add3(cam, scale3(rd, t))
	if abs32(p.x) > halfW || abs32(p.y-0.28) > halfH {
		return 0, 0, false
	}
	u := (p.x + halfW) / (2 * halfW)
	v := (halfH - (p.y - 0.28)) / (2 * halfH)
	if rd.z > 0 {
		u = 1 - u
	}
	return u, v, true
}

func (a *app) intersectPanel(cam, rd vec3, halfW, halfH, offset float32) float32 {
	if abs32(rd.z) <= 0.02 {
		return -1
	}
	t := (offset - cam.z) / rd.z
	p := add3(cam, scale3(rd, t))
	if t > 0.01 && abs32(p.x) <= halfW && abs32(p.y-0.28) <= halfH {
		return t
	}
	return -1
}

type vec3 struct{ x, y, z float32 }

func add3(a, b vec3) vec3           { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func scale3(a vec3, s float32) vec3 { return vec3{a.x * s, a.y * s, a.z * s} }
func cross3(a, b vec3) vec3 {
	return vec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func normalize3(a vec3) vec3 {
	l := float32(math.Sqrt(float64(a.x*a.x + a.y*a.y + a.z*a.z)))
	if l == 0 {
		return vec3{}
	}
	return scale3(a, 1/l)
}
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }

func main() {
	a := newApp()
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:          "MyGo WebGPU Flagscape",
			Width:          1440,
			Height:         900,
			MinWidth:       1080,
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
