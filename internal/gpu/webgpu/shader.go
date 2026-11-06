package webgpu

// sceneShader is the beginning of the WGSL port of the native scene
// shader. It reads the same fourteen-float4 gpu.Instance stream as the
// Metal, Direct3D and OpenGL renderers. This stage covers solid rounded
// fills, linear gradients, solid borders, projective transforms, image
// batches and grayscale mask glyphs; unsupported scene features are
// rejected by Renderer.Render until their paths are ported.
const sceneShader = `
struct Globals {
	size: vec4<f32>,
}

struct Instance {
	rect: vec4<f32>,
	radii: vec4<f32>,
	inner: vec4<f32>,
	color: vec4<f32>,
	color2: vec4<f32>,
	border: vec4<f32>,
	grad: vec4<f32>,
	uv: vec4<f32>,
	clip: vec4<f32>,
	clip_radii: vec4<f32>,
	params: vec4<f32>,
	transform0: vec4<f32>,
	transform1: vec4<f32>,
	transform2: vec4<f32>,
}

struct VertexOut {
	@builtin(position) position: vec4<f32>,
	@location(0) local: vec2<f32>,
	@location(1) color: vec4<f32>,
	@location(2) @interpolate(flat) rect: vec4<f32>,
	@location(3) @interpolate(flat) radii: vec4<f32>,
	@location(4) @interpolate(flat) params: vec4<f32>,
	@location(5) tex: vec2<f32>,
	@location(6) @interpolate(flat) clip: vec4<f32>,
	@location(7) @interpolate(flat) color2: vec4<f32>,
	@location(8) @interpolate(flat) grad: vec4<f32>,
	@location(9) @interpolate(flat) border: vec4<f32>,
	@location(10) @interpolate(flat) inner: vec4<f32>,
	@location(11) @interpolate(flat) uv_border: vec4<f32>,
}

@group(0) @binding(0) var<uniform> globals: Globals;
@group(0) @binding(1) var<storage, read> instances: array<Instance>;
@group(0) @binding(2) var image_tex: texture_2d<f32>;
@group(0) @binding(3) var mask_tex: texture_2d<f32>;
@group(0) @binding(4) var color_tex: texture_2d<f32>;
@group(0) @binding(5) var texture_sampler: sampler;

fn frame_h(inst: Instance, p: vec2<f32>) -> vec3<f32> {
	return vec3<f32>(
		dot(inst.transform0.xyz, vec3<f32>(p, 1.0)),
		dot(inst.transform1.xyz, vec3<f32>(p, 1.0)),
		dot(vec3<f32>(inst.transform2.x, inst.transform2.y, 1.0), vec3<f32>(p, 1.0)),
	);
}

@vertex
fn vs(
	@builtin(vertex_index) vertex_index: u32,
	@builtin(instance_index) instance_index: u32,
) -> VertexOut {
	let inst = instances[instance_index];
	let corner = vec2<f32>(f32(vertex_index & 1u), f32(vertex_index >> 1u));
	var r = inst.rect;
	let kind = inst.params.x;
	if (kind < 0.5 || kind > 5.5) {
		r = vec4<f32>(r.xy - vec2<f32>(1.0), r.zw + vec2<f32>(2.0));
	}
	var p = r.xy + corner * r.zw;
	var h = frame_h(inst, p);
	if (inst.transform0.w > 0.5 && h.z > 0.0) {
		let s = h.xy / h.z;
		let a = (inst.transform0.x - s.x * inst.transform2.x) / h.z;
		let c = (inst.transform0.y - s.x * inst.transform2.y) / h.z;
		let b = (inst.transform1.x - s.y * inst.transform2.x) / h.z;
		let d = (inst.transform1.y - s.y * inst.transform2.y) / h.z;
		let e = vec2<f32>(abs(d) + abs(c), abs(b) + abs(a)) / abs(a * d - b * c);
		r = vec4<f32>(r.xy - e, r.zw + vec2<f32>(2.0) * e);
		p = r.xy + corner * r.zw;
		h = frame_h(inst, p);
	}
	var out: VertexOut;
	out.position = vec4<f32>(
		h.xy / globals.size.xy * vec2<f32>(2.0, -2.0) + h.z * vec2<f32>(-1.0, 1.0),
		0.0,
		h.z,
	);
	out.local = p;
	out.color = inst.color;
	out.rect = inst.rect;
	out.radii = inst.radii;
	out.params = inst.params;
	out.tex = inst.uv.xy + (p - inst.rect.xy) / inst.rect.zw * (inst.uv.zw - inst.uv.xy);
	out.clip = inst.clip;
	out.color2 = inst.color2;
	out.grad = inst.grad;
	out.border = inst.border;
	out.inner = inst.inner;
	out.uv_border = inst.uv;
	return out;
}

fn sd_round_rect(p: vec2<f32>, rect: vec4<f32>, radii: vec4<f32>) -> f32 {
	let half = rect.zw * 0.5;
	let q = p - rect.xy - half;
	var r: f32;
	if (q.x < 0.0) {
		r = select(radii.w, radii.x, q.y < 0.0);
	} else {
		r = select(radii.z, radii.y, q.y < 0.0);
	}
	let a = abs(q) - half + vec2<f32>(r);
	return length(max(a, vec2<f32>(0.0))) + min(max(a.x, a.y), 0.0) - r;
}

fn coverage(d: f32) -> f32 {
	return clamp(0.5 - d, 0.0, 1.0);
}

fn clip_coverage(p: vec2<f32>, clip: vec4<f32>) -> f32 {
	if (clip.z <= 0.0 || clip.w <= 0.0) {
		return 1.0;
	}
	let c = saturate(min(clip.xy + clip.zw, p + vec2<f32>(0.5)) - max(clip.xy, p - vec2<f32>(0.5)));
	return c.x * c.y;
}

fn tint(c: vec4<f32>, t: vec4<f32>) -> vec4<f32> {
	return c * vec4<f32>(t.rgb * t.a, t.a);
}

fn premul(c: vec4<f32>) -> vec4<f32> {
	return vec4<f32>(c.rgb * c.a, c.a);
}

fn paint(p: vec2<f32>, mode: f32, c1: vec4<f32>, c2: vec4<f32>, grad: vec4<f32>) -> vec4<f32> {
	if (mode > 1.5) {
		return premul(c1);
	}
	let d = grad.zw - grad.xy;
	let denom = max(dot(d, d), 1e-6);
	let t = saturate(dot(p - grad.xy, d) / denom);
	return mix(premul(c1), premul(c2), t);
}

@fragment
fn fs(in: VertexOut) -> @location(0) vec4<f32> {
	let kind = in.params.x;
	let shape = coverage(sd_round_rect(in.local, in.rect, max(in.radii, vec4<f32>(0.0))));
	let clip = clip_coverage(in.local, in.clip);
	let opacity = in.params.w * shape * clip;
	if (kind < 0.5) {
		var c = paint(in.local, in.params.z, in.color, in.color2, in.grad) * opacity;
		let bw = in.uv_border;
		if (any(bw > vec4<f32>(0.0))) {
			let inner = vec4<f32>(
				in.rect.xy + vec2<f32>(bw.w, bw.x),
				in.rect.zw - vec2<f32>(bw.y + bw.w, bw.x + bw.z),
			);
			let innerShape = coverage(sd_round_rect(in.local, inner, max(in.inner, vec4<f32>(0.0))));
			let borderShape = saturate(shape - innerShape);
			let border = premul(in.border) * borderShape * in.params.w * clip;
			c = border + c * (1.0 - border.a);
		}
		return c;
	}
	if (kind < 2.5) {
		let a = textureSampleLevel(mask_tex, texture_sampler, clamp(in.tex, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).r;
		return paint(in.local, in.params.z, in.color, in.color2, in.grad) * (a * opacity);
	}
	if (kind < 3.5) {
		let c = textureSampleLevel(color_tex, texture_sampler, clamp(in.tex, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0);
		return c * in.color.a * opacity;
	}
	if (kind < 4.5) {
		var c = textureSampleLevel(image_tex, texture_sampler, clamp(in.tex, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0);
		if (in.params.y > 0.5) {
			c = vec4<f32>(vec3<f32>(dot(c.rgb, vec3<f32>(0.2126, 0.7152, 0.0722))), c.a);
		}
		return tint(c, in.color) * opacity;
	}
	if (kind < 5.5) {
		let a = textureSampleLevel(color_tex, texture_sampler, clamp(in.tex, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0);
		let cov = dot(a.rgb, vec3<f32>(0.333333));
		return vec4<f32>(in.color.rgb * in.color.a, in.color.a) * (cov * opacity);
	}
	return vec4<f32>(0.0);
}
`
