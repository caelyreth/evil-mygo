fn blur_at(p: vec2<i32>, size: vec2<i32>) -> vec4<f32> {
	return textureLoad(backdrop_tex, clamp(p, vec2<i32>(0), size - 1), 0);
}

fn blur_sample(e: Effect, q: vec2<f32>) -> vec4<f32> {
	let p = effect_frame_point(e, q);
	let u = (p - e.area.xy) / e.params.z - 0.5;
	let f = floor(u);
	let w = u - f;
	let at = vec2<i32>(f);
	let size = vec2<i32>(e.area.zw);
	let top = blur_at(at, size) * (1.0 - w.x) + blur_at(at + vec2<i32>(1, 0), size) * w.x;
	let bottom = blur_at(at + vec2<i32>(0, 1), size) * (1.0 - w.x) + blur_at(at + vec2<i32>(1), size) * w.x;
	return top * (1.0 - w.y) + bottom * w.y;
}

fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	let d = e.p0.zw - e.p0.xy;
	let l = d.x * d.x + d.y * d.y;
	var t = 0.0;
	if (l > 0.0) {
		t = clamp(((p.x - e.p0.x) * d.x + (p.y - e.p0.y) * d.y) / max(l, 0.0001), 0.0, 1.0);
	}
	let blur = e.p1.x * (1.0 - t) + e.p1.y * t;
	let w = clamp((blur - e.p1.z) / (e.p1.w - e.p1.z), 0.0, 1.0);
	if (w <= 0.0) {
		return vec4<f32>(0.0);
	}
	var c = blur_sample(e, p);
	let lum = 0.2126 * c.r + 0.7152 * c.g + 0.0722 * c.b;
	c = vec4<f32>(
		clamp(vec3<f32>(lum) + (c.rgb - vec3<f32>(lum)) * (1.0 + e.p2.x) + vec3<f32>(e.p2.y * c.a),
			vec3<f32>(0.0), vec3<f32>(c.a)),
		c.a,
	);
	c += (vec4<f32>(e.p3.rgb, 1.0) - c) * e.p2.z;
	return c * w;
}
