fn glass_lens(t: f32, bezel: f32) -> f32 {
	if (bezel <= 0.0 || t >= bezel) {
		return 0.0;
	}
	let u = 1.0 - max(t, 0.0) / bezel;
	let v = 1.0 - u * u * u * u;
	let a = sqrt(v * sqrt(v));
	let b = u * u * u;
	let n = sqrt(a * a + b * b);
	let s = b / n;
	let c = a / n;
	let st = s / 1.5;
	let ct = sqrt(1.0 - st * st);
	return (s * ct - c * st) / (c * ct + s * st) / 1.1180340;
}

fn glass_normal(p: vec2<f32>, rect: vec4<f32>, radii: vec4<f32>) -> vec2<f32> {
	let h = rect.zw * 0.5;
	let q = p - rect.xy - h;
	var r: f32;
	if (q.x < 0.0) {
		r = select(radii.w, radii.x, q.y < 0.0);
	} else {
		r = select(radii.z, radii.y, q.y < 0.0);
	}
	r = min(r * 1.5, min(h.x, h.y));
	let a = abs(q) - (h - vec2<f32>(r));
	if (a.x > 0.0 || a.y > 0.0) {
		let m = max(a, vec2<f32>(0.0));
		return sign(q) * m / sqrt(dot(m, m));
	}
	return select(vec2<f32>(0.0, sign(q.y)), vec2<f32>(sign(q.x), 0.0), a.x > a.y);
}

fn glass_tone(c: vec3<f32>, tone: vec4<f32>) -> vec3<f32> {
	let l = 0.2126 * c.r + 0.7152 * c.g + 0.0722 * c.b;
	let t = tone.x + (tone.y - tone.x) * (1.0 - pow(max(1.0 - l, 0.0), tone.z));
	let k = (tone.y - tone.x) * tone.w;
	return clamp(t + (c - vec3<f32>(l)) * k, vec3<f32>(0.0), vec3<f32>(1.0));
}

fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	let t = max(-effect_sd(p, e.rect, e.radii), 0.0);
	let bezel = e.p0.x;
	let rim_width = e.p0.w;
	var n = vec2<f32>(0.0);
	if (t < bezel || t < 3.0 * rim_width) {
		n = glass_normal(p, e.rect, e.radii);
	}
	var q = p;
	if (t < bezel) {
		q -= n * (e.p0.y * glass_lens(t, bezel));
	}
	var c = glass_tone(effect_sample_backdrop(e, q), e.p2);
	c += (e.p1.rgb - c) * e.p1.a;
	if (e.p0.z > 0.0 && t < 3.0 * rim_width) {
		let r = t / rim_width;
		let a = e.p0.z * exp(-r * r);
		let l = abs(n.x * e.p3.x + n.y * e.p3.y);
		c += (vec3<f32>(l) - c) * a;
	}
	return vec4<f32>(c, 1.0);
}
