//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"errors"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/ui"
)

const membraneShader = `
struct Uniforms {
	viewport: vec4<f32>,
	camera: vec4<f32>,
	state: vec4<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var source: texture_2d<f32>;
@group(0) @binding(2) var sourceSampler: sampler;

const panelLift: f32 = 0.28;
const panelHalfH: f32 = 0.70;
const panelThickness: f32 = 0.028;
const groundY: f32 = -0.74;
const fieldOfView: f32 = 0.74;

fn membraneWave(u: f32, v: f32, t: f32, amplitude: f32) -> f32 {
	let a = sin(6.1 * u + 0.82 * t) * sin(3.2 * v - 0.46 * t);
	let b = sin(12.4 * u - 1.27 * t) * sin(6.3 * v + 0.31 * t);
	return amplitude * (0.105 * a + 0.034 * b);
}

fn membraneNormal(u: f32, v: f32, t: f32, amplitude: f32, aspect: f32) -> vec3<f32> {
	let e = 0.0015;
	let halfW = panelHalfH * aspect;
	let zu = (membraneWave(u + e, v, t, amplitude) - membraneWave(u - e, v, t, amplitude)) / (2.0 * e);
	let zv = (membraneWave(u, v + e, t, amplitude) - membraneWave(u, v - e, t, amplitude)) / (2.0 * e);
	return normalize(vec3<f32>(-zu * 2.0 * panelHalfH, 2.0 * halfW * zv, 4.0 * halfW * panelHalfH));
}

struct Hit {
	t: f32,
	uv: vec2<f32>,
	p: vec3<f32>,
	n: vec3<f32>,
	ok: bool,
	front: bool,
	edge: bool,
}

fn floorDepth(cam: vec3<f32>, rd: vec3<f32>) -> f32 {
	if (rd.y >= -0.001) {
		return -1.0;
	}
	return (groundY - cam.y) / rd.y;
}

fn rayDirection(uv: vec2<f32>, fwd: vec3<f32>, right: vec3<f32>, up: vec3<f32>, aspect: f32) -> vec3<f32> {
	var ndc = uv * 2.0 - vec2<f32>(1.0);
	ndc.y = -ndc.y;
	return normalize(fwd + right * ndc.x * fieldOfView * aspect + up * ndc.y * fieldOfView);
}

fn emptyHit() -> Hit {
	return Hit(0.0, vec2<f32>(0.0), vec3<f32>(0.0), vec3<f32>(0.0, 0.0, 1.0), false, false, false);
}

fn closest(a: Hit, b: Hit) -> Hit {
	if (!a.ok) {
		return b;
	}
	if (!b.ok) {
		return a;
	}
	if (b.t < a.t) {
		return b;
	}
	return a;
}

fn intersectSheet(cam: vec3<f32>, rd: vec3<f32>, aspect: f32, waveTime: f32, amplitude: f32, front: bool) -> Hit {
	var hit = emptyHit();
	if (abs(rd.z) < 0.015) {
		return hit;
	}
	let halfW = panelHalfH * aspect;
	let offset = select(-panelThickness, 0.0, front);
	var t = (offset - cam.z) / rd.z;
	var p = cam;
	var uv = vec2<f32>(0.0);
	for (var i: i32 = 0; i < 9; i = i + 1) {
		p = cam + rd * t;
		uv = vec2<f32>(
			(p.x + halfW) / (2.0 * halfW),
			(panelLift + panelHalfH - p.y) / (2.0 * panelHalfH),
		);
		let z = membraneWave(clamp(uv.x, 0.0, 1.0), clamp(uv.y, 0.0, 1.0), waveTime, amplitude) + offset;
		t = (z - cam.z) / rd.z;
	}
	p = cam + rd * t;
	uv = vec2<f32>(
		(p.x + halfW) / (2.0 * halfW),
		(panelLift + panelHalfH - p.y) / (2.0 * panelHalfH),
	);
	if (t <= 0.01 || uv.x < 0.0 || uv.x > 1.0 || uv.y < 0.0 || uv.y > 1.0) {
		return hit;
	}
	hit.t = t;
	hit.uv = uv;
	hit.p = p;
	hit.n = membraneNormal(uv.x, uv.y, waveTime, amplitude, aspect);
	if (!front) {
		hit.n = -hit.n;
	}
	hit.ok = true;
	hit.front = front;
	hit.edge = false;
	return hit;
}

fn intersectSideU(cam: vec3<f32>, rd: vec3<f32>, aspect: f32, waveTime: f32, amplitude: f32, side: bool) -> Hit {
	var hit = emptyHit();
	if (abs(rd.x) < 0.001) {
		return hit;
	}
	let halfW = panelHalfH * aspect;
	let x = select(-halfW, halfW, side);
	let t = (x - cam.x) / rd.x;
	let p = cam + rd * t;
	let v = (panelLift + panelHalfH - p.y) / (2.0 * panelHalfH);
	if (t <= 0.01 || v < 0.0 || v > 1.0) {
		return hit;
	}
	let u = select(0.0, 1.0, side);
	let front = membraneWave(u, clamp(v, 0.0, 1.0), waveTime, amplitude);
	if (p.z < front - panelThickness - 0.001 || p.z > front + 0.001) {
		return hit;
	}
	hit.t = t;
	hit.uv = vec2<f32>(u, clamp(v, 0.0, 1.0));
	hit.p = p;
	hit.n = vec3<f32>(select(-1.0, 1.0, side), 0.0, 0.0);
	hit.ok = true;
	hit.front = false;
	hit.edge = true;
	return hit;
}

fn intersectSideV(cam: vec3<f32>, rd: vec3<f32>, aspect: f32, waveTime: f32, amplitude: f32, side: bool) -> Hit {
	var hit = emptyHit();
	if (abs(rd.y) < 0.001) {
		return hit;
	}
	let y = select(panelLift - panelHalfH, panelLift + panelHalfH, side);
	let t = (y - cam.y) / rd.y;
	let p = cam + rd * t;
	let halfW = panelHalfH * aspect;
	let u = (p.x + halfW) / (2.0 * halfW);
	if (t <= 0.01 || u < 0.0 || u > 1.0) {
		return hit;
	}
	let v = select(0.0, 1.0, side);
	let front = membraneWave(clamp(u, 0.0, 1.0), v, waveTime, amplitude);
	if (p.z < front - panelThickness - 0.001 || p.z > front + 0.001) {
		return hit;
	}
	hit.t = t;
	hit.uv = vec2<f32>(clamp(u, 0.0, 1.0), v);
	hit.p = p;
	hit.n = vec3<f32>(0.0, select(-1.0, 1.0, side), 0.0);
	hit.ok = true;
	hit.front = false;
	hit.edge = true;
	return hit;
}

fn environment(rd: vec3<f32>) -> vec3<f32> {
	let h = smoothstep(-0.25, 0.72, rd.y);
	let sky = mix(vec3<f32>(0.105, 0.115, 0.125), vec3<f32>(0.70, 0.72, 0.73), h);
	let sun = pow(max(dot(rd, normalize(vec3<f32>(0.38, 0.42, 0.82))), 0.0), 64.0);
	return sky + vec3<f32>(0.12) * sun;
}

fn floorColor(p: vec3<f32>) -> vec3<f32> {
	let g = abs(fract(p.xz * 4.0) - vec2<f32>(0.5));
	let grid = 1.0 - smoothstep(0.0, 0.025, min(g.x, g.y));
	let clarity = 1.0 - smoothstep(1.0, 5.0, length(p.xz));
	let col = mix(vec3<f32>(0.085), vec3<f32>(0.27), clarity);
	return col + vec3<f32>(0.20) * grid * clarity;
}

fn shadeSurface(base: vec3<f32>, n: vec3<f32>, rd: vec3<f32>, edge: bool) -> vec3<f32> {
	let light = normalize(vec3<f32>(-0.48, 0.72, 0.50));
	let view = -rd;
	let diffuse = max(dot(n, light), 0.0);
	let spec = pow(max(dot(n, normalize(light + view)), 0.0), 42.0);
	let wrap = mix(0.56, 1.0, smoothstep(0.0, 0.28, diffuse));
	let specular = select(0.14, 0.035, edge);
	let rim = pow(1.0 - max(dot(n, view), 0.0), 3.0) * 0.04;
	return base * wrap + vec3<f32>(spec * specular + rim);
}

@vertex
fn vertexMain(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
	var positions = array<vec2<f32>, 3>(
		vec2<f32>(-1.0, -1.0),
		vec2<f32>(3.0, -1.0),
		vec2<f32>(-1.0, 3.0),
	);
	return vec4<f32>(positions[index], 0.0, 1.0);
}

@fragment
fn fragmentMain(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let viewport = uniforms.viewport.xy;
	let cam = uniforms.camera.xyz;
	let yaw = uniforms.camera.w;
	let pitch = uniforms.state.x;
	let waveTime = uniforms.state.y;
	let amplitude = uniforms.state.z;
	let aspect = max(uniforms.state.w, 0.01);
	let fwd = normalize(vec3<f32>(
		sin(yaw) * cos(pitch),
		sin(pitch),
		-cos(yaw) * cos(pitch),
	));
	let right = normalize(cross(fwd, vec3<f32>(0.0, 1.0, 0.0)));
	let up = normalize(cross(right, fwd));
	var total = vec3<f32>(0.0);
	for (var sy: i32 = 0; sy < 2; sy = sy + 1) {
		for (var sx: i32 = 0; sx < 2; sx = sx + 1) {
			let jitter = (vec2<f32>(f32(sx), f32(sy)) + vec2<f32>(0.5)) * 0.5;
			let screen = (position.xy + jitter) / viewport;
			let rd = rayDirection(screen, fwd, right, up, aspect);
			var col = environment(rd);
			let tf = floorDepth(cam, rd);
			var hit = intersectSheet(cam, rd, aspect, waveTime, amplitude, true);
			hit = closest(hit, intersectSheet(cam, rd, aspect, waveTime, amplitude, false));
			hit = closest(hit, intersectSideU(cam, rd, aspect, waveTime, amplitude, false));
			hit = closest(hit, intersectSideU(cam, rd, aspect, waveTime, amplitude, true));
			hit = closest(hit, intersectSideV(cam, rd, aspect, waveTime, amplitude, false));
			hit = closest(hit, intersectSideV(cam, rd, aspect, waveTime, amplitude, true));
			let floorVisible = tf > 0.01 && (!hit.ok || tf < hit.t);
			if (floorVisible) {
				col = floorColor(cam + rd * tf);
			}
			if (hit.ok && (!floorVisible || hit.t < tf)) {
				var base = vec3<f32>(0.15);
				if (hit.edge) {
					base = vec3<f32>(0.16, 0.17, 0.18);
				} else if (hit.front) {
					let tex = textureSample(source, sourceSampler, clamp(hit.uv, vec2<f32>(0.0), vec2<f32>(1.0)));
					base = vec3<f32>(dot(tex.rgb, vec3<f32>(0.299, 0.587, 0.114)));
				} else {
					base = vec3<f32>(0.30, 0.31, 0.32);
				}
				col = shadeSurface(base, hit.n, rd, hit.edge);
			}
			let q = screen - vec2<f32>(0.5);
			col *= 1.0 - dot(q, q) * 0.08;
			total += col;
		}
	}
	return vec4<f32>(total * 0.25, 1.0);
}
`

type membraneRenderer struct {
	pipeline   *wgpu.RenderPipeline
	uniforms   *wgpu.Buffer
	sampler    *wgpu.Sampler
	bind       *wgpu.BindGroup
	sourceView *wgpu.TextureView
	sourceAt   *wgpu.Texture
}

func (m *membraneRenderer) draw(f ui.WebGPUFrame, camX, camY, camZ, yaw, pitch, waveTime, amplitude, aspect float32) error {
	if f.Device == nil || f.Encoder == nil || f.Texture == nil || f.SourceTexture == nil {
		return nil
	}
	if m.pipeline == nil {
		module, err := f.Device.CreateShaderModuleWGSL(membraneShader)
		if err != nil {
			return err
		}
		m.pipeline, err = f.Device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
			Label:  "Layer membrane panel",
			Vertex: wgpu.VertexState{Module: module, EntryPoint: "vertexMain"},
			Primitive: wgpu.PrimitiveState{
				Topology: wgpu.PrimitiveTopologyTriangleList,
				CullMode: wgpu.CullModeNone,
			},
			Multisample: wgpu.MultisampleState{Count: 1},
			Fragment: &wgpu.FragmentState{
				Module: module, EntryPoint: "fragmentMain",
				Targets: []wgpu.ColorTargetState{{
					Format: wgpu.TextureFormatRGBA8Unorm, WriteMask: wgpu.ColorWriteMaskAll,
				}},
			},
		})
		module.Release()
		if err != nil {
			return err
		}
		m.uniforms, err = f.Device.CreateBuffer(&wgpu.BufferDescriptor{
			Label: "Layer membrane uniforms",
			Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst,
			Size:  48,
		})
		if err != nil {
			return err
		}
		m.sampler, err = f.Device.CreateLinearSampler()
		if err != nil {
			return err
		}
	}
	if m.bind == nil || m.sourceAt != f.SourceTexture {
		if m.bind != nil {
			m.bind.Release()
			m.bind = nil
		}
		if m.sourceView != nil {
			m.sourceView.Release()
			m.sourceView = nil
		}
		view, err := f.SourceTexture.CreateView(nil)
		if err != nil {
			return err
		}
		layout := m.pipeline.GetBindGroupLayout(0)
		if layout == nil {
			view.Release()
			return errors.New("layer-membrane pipeline has no bind group layout")
		}
		bind, err := f.Device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
			wgpu.BufferBindingEntry(0, m.uniforms, 0, 48),
			wgpu.TextureBindingEntry(1, view),
			wgpu.SamplerBindingEntry(2, m.sampler),
		})
		layout.Release()
		if err != nil {
			view.Release()
			return err
		}
		m.bind, m.sourceView, m.sourceAt = bind, view, f.SourceTexture
	}
	values := [12]float32{
		float32(f.Width), float32(f.Height), float32(f.SourceWidth), float32(f.SourceHeight),
		camX, camY, camZ, yaw, pitch, waveTime, amplitude, max(aspect, 0.01),
	}
	if err := f.Device.Queue().WriteBuffer(m.uniforms, 0, unsafe.Slice(
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
		Label: "Layer membrane pass",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore,
			ClearValue: wgpu.Color{R: 0.03, G: 0.04, B: 0.05, A: 1},
		}},
	})
	if err != nil {
		return err
	}
	pass.SetPipeline(m.pipeline)
	pass.SetBindGroup(0, m.bind, nil)
	pass.Draw(3, 1, 0, 0)
	pass.End()
	pass.Release()
	return nil
}
