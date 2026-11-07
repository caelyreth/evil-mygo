struct EffectInstance {
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

struct Effect {
	size: vec4<f32>,
	rect: vec4<f32>,
	radii: vec4<f32>,
	p0: vec4<f32>,
	p1: vec4<f32>,
	p2: vec4<f32>,
	p3: vec4<f32>,
	p4: vec4<f32>,
	area: vec4<f32>,
	clip: vec4<f32>,
	clip_radii: vec4<f32>,
	transform0: vec4<f32>,
	transform1: vec4<f32>,
	transform2: vec4<f32>,
	params: vec4<f32>,
	inner: vec4<f32>,
	uv: vec4<f32>,
}

struct EffectVary {
	@builtin(position) position: vec4<f32>,
	@location(0) local: vec2<f32>,
	@location(1) tex: vec2<f32>,
	@location(2) world: vec3<f32>,
	@location(3) @interpolate(flat) rect: vec4<f32>,
	@location(4) @interpolate(flat) radii: vec4<f32>,
	@location(5) @interpolate(flat) inner: vec4<f32>,
	@location(6) @interpolate(flat) clip: vec4<f32>,
	@location(7) @interpolate(flat) clip_radii: vec4<f32>,
	@location(8) @interpolate(flat) transform0: vec4<f32>,
	@location(9) @interpolate(flat) params: vec4<f32>,
	@location(10) @interpolate(flat) grad: vec4<f32>,
	@location(11) @interpolate(flat) clip_mode: f32,
}

@group(0) @binding(0) var<storage, read> effect_params: Effect;
@group(0) @binding(1) var<storage, read> effect_instances: array<EffectInstance>;
@group(0) @binding(2) var T0: texture_2d<f32>;
@group(0) @binding(3) var T1: texture_2d<f32>;
@group(0) @binding(4) var T2: texture_2d<f32>;
@group(0) @binding(5) var T3: texture_2d<f32>;
@group(0) @binding(6) var effect_sampler: sampler;
@group(0) @binding(7) var clip_tex: texture_2d<f32>;
@group(0) @binding(8) var mask_tex: texture_2d<f32>;
@group(0) @binding(9) var<storage, read> shader_uniforms: array<u32>;
@group(0) @binding(10) var backdrop_tex: texture_2d<f32>;

fn effect_frame_h(inst: EffectInstance, p: vec2<f32>) -> vec3<f32> {
	return vec3<f32>(
		dot(inst.transform0.xyz, vec3<f32>(p, 1.0)),
		dot(inst.transform1.xyz, vec3<f32>(p, 1.0)),
		dot(vec3<f32>(inst.transform2.xy, 1.0 - 2.0 * inst.transform2.z), vec3<f32>(p, 1.0)),
	);
}

@vertex
fn effect_vs(
	@builtin(vertex_index) vertex_index: u32,
	@builtin(instance_index) instance_index: u32,
) -> EffectVary {
	let inst = effect_instances[instance_index];
	let corner = vec2<f32>(f32(vertex_index & 1u), f32(vertex_index >> 1u));
	var r = vec4<f32>(inst.rect.xy - vec2<f32>(1.0), inst.rect.zw + vec2<f32>(2.0));
	var p = r.xy + corner * r.zw;
	var h = effect_frame_h(inst, p);
	if (inst.transform0.w > 0.5 && h.z > 0.0) {
		let s = h.xy / h.z;
		let a = (inst.transform0.x - s.x * inst.transform2.x) / h.z;
		let c = (inst.transform0.y - s.x * inst.transform2.y) / h.z;
		let b = (inst.transform1.x - s.y * inst.transform2.x) / h.z;
		let d = (inst.transform1.y - s.y * inst.transform2.y) / h.z;
		let e = vec2<f32>(abs(d) + abs(c), abs(b) + abs(a)) / abs(a * d - b * c);
		r = vec4<f32>(r.xy - e, r.zw + vec2<f32>(2.0) * e);
		p = r.xy + corner * r.zw;
		h = effect_frame_h(inst, p);
	}
	var out: EffectVary;
	out.position = vec4<f32>(
		h.xy / effect_params.size.xy * vec2<f32>(2.0, -2.0) + h.z * vec2<f32>(-1.0, 1.0),
		0.0,
		h.z,
	);
	out.local = p;
	out.tex = inst.uv.xy + (p - inst.rect.xy) / inst.rect.zw * (inst.uv.zw - inst.uv.xy);
	out.world = h;
	out.rect = inst.rect;
	out.radii = inst.radii;
	out.inner = inst.inner;
	out.clip = inst.clip;
	out.clip_radii = inst.clip_radii;
	out.transform0 = inst.transform0;
	out.params = inst.params;
	out.grad = inst.grad;
	out.clip_mode = inst.transform1.w;
	return out;
}

fn effect_sd(p: vec2<f32>, rect: vec4<f32>, radii: vec4<f32>) -> f32 {
	let half = rect.zw * 0.5;
	let q = p - rect.xy - half;
	// Effect corners are shown circular; the GPU scene shader keeps the
	// continuous curve for the element's own shape.
	var r: f32;
	if (q.x < 0.0) {
		r = select(radii.w, radii.x, q.y < 0.0);
	} else {
		r = select(radii.z, radii.y, q.y < 0.0);
	}
	let d = abs(q) - half + vec2<f32>(r);
	return length(max(d, vec2<f32>(0.0))) + min(max(d.x, d.y), 0.0) - r;
}

fn effect_coverage(p: vec2<f32>, rect: vec4<f32>, radii: vec4<f32>) -> f32 {
	let rr = abs(radii);
	let q = p - rect.xy - rect.zw * 0.5;
	let r = select(
		select(rr.z, rr.y, q.y < 0.0),
		select(rr.w, rr.x, q.y < 0.0),
		q.x < 0.0,
	);
	if (r > 0.0) {
		return clamp(0.5 - effect_sd(p, rect, rr), 0.0, 1.0);
	}
	let c = saturate(min(rect.xy + rect.zw, p + vec2<f32>(0.5)) - max(rect.xy, p - vec2<f32>(0.5)));
	return c.x * c.y;
}

fn effect_clip(in: EffectVary) -> f32 {
	if (in.clip_mode > 2.5) {
		return 1.0;
	}
	let world = in.world.xy / in.world.z;
	if (in.clip_mode < 0.5) {
		return effect_coverage(world, in.clip, in.clip_radii);
	}
	if (in.clip_mode > 1.5) {
		return effect_coverage(in.local, in.clip, vec4<f32>(0.0)) *
			textureSampleLevel(clip_tex, effect_sampler,
				clamp((world - in.clip_radii.xy) / in.clip_radii.zw, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).a;
	}
	return textureSampleLevel(clip_tex, effect_sampler,
		clamp((world - in.clip.xy) / in.clip.zw, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).a;
}

fn effect_unpremul(c: vec4<f32>) -> vec3<f32> {
	return select(vec3<f32>(0.0), c.rgb / c.a, c.a > 0.0);
}

fn effect_text_coverage(a: f32, c: vec3<f32>, contrast: f32, boost: f32, g: vec4<f32>) -> f32 {
	let k = contrast * saturate(3.0 - 4.0 * dot(c, vec3<f32>(0.30, 0.59, 0.11))) + boost;
	let adjusted = a * (k + 1.0) / (a * k + 1.0);
	let f = dot(c, vec3<f32>(0.25, 0.5, 0.25));
	return saturate(adjusted + adjusted * (1.0 - adjusted) * ((g.x * f + g.y) * adjusted + (g.z * f + g.w)));
}

fn effect_frame_point(e: Effect, q: vec2<f32>) -> vec2<f32> {
	let h = vec3<f32>(
		dot(e.transform0.xyz, vec3<f32>(q, 1.0)),
		dot(e.transform1.xyz, vec3<f32>(q, 1.0)),
		dot(vec3<f32>(e.transform2.xy, 1.0 - 2.0 * e.transform2.z), vec3<f32>(q, 1.0)),
	);
	return h.xy / h.z;
}

fn effect_sample_backdrop_rgba(e: Effect, q: vec2<f32>) -> vec4<f32> {
	let p = effect_frame_point(e, q);
	let u = (p - e.area.xy) / e.params.z - 0.5;
	let f = floor(u);
	let w = u - f;
	let at = vec2<i32>(f);
	let size = vec2<i32>(e.area.zw);
	let top = textureLoad(backdrop_tex, clamp(at, vec2<i32>(0), size - 1), 0) * (1.0 - w.x)
		+ textureLoad(backdrop_tex, clamp(at + vec2<i32>(1, 0), vec2<i32>(0), size - 1), 0) * w.x;
	let bottom = textureLoad(backdrop_tex, clamp(at + vec2<i32>(0, 1), vec2<i32>(0), size - 1), 0) * (1.0 - w.x)
		+ textureLoad(backdrop_tex, clamp(at + vec2<i32>(1), vec2<i32>(0), size - 1), 0) * w.x;
	return top * (1.0 - w.y) + bottom * w.y;
}

fn effect_sample_backdrop(e: Effect, q: vec2<f32>) -> vec3<f32> {
	return effect_sample_backdrop_rgba(e, q).rgb;
}

// effect

@fragment
fn effect_fs(in: EffectVary) -> @location(0) vec4<f32> {
	// Keep every optional effect resource in the pipeline's auto layout,
	// even when the shader does not use it.
	let keep = textureSampleLevel(T0, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(T1, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(T2, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(T3, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(clip_tex, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(mask_tex, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ textureSampleLevel(backdrop_tex, effect_sampler, vec2<f32>(0.5), 0.0).x
		+ bitcast<f32>(shader_uniforms[0]);
	if (effect_params.params.x < -1.0) {
		return vec4<f32>(keep);
	}
	var c = effect(in.local, effect_params);
	var coverage = effect_coverage(in.local, in.rect, in.radii);
	if (in.params.y > 0.5) {
		let mask = textureSampleLevel(mask_tex, effect_sampler,
			clamp(in.tex, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).r;
		coverage *= effect_text_coverage(mask, effect_unpremul(c), in.inner.x, in.inner.y, in.radii);
	}
	return c * (coverage * effect_clip(in) * in.params.w);
}
