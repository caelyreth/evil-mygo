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
	@location(12) world: vec3<f32>,
	@location(13) @interpolate(flat) clip_mode: f32,
	@location(14) @interpolate(flat) clip_radii: vec4<f32>,
	@location(15) @interpolate(flat) transform0: vec4<f32>,
}

@group(0) @binding(0) var<uniform> globals: Globals;
@group(0) @binding(1) var<storage, read> instances: array<Instance>;
@group(0) @binding(2) var image_tex: texture_2d<f32>;
@group(0) @binding(3) var mask_tex: texture_2d<f32>;
@group(0) @binding(4) var color_tex: texture_2d<f32>;
@group(0) @binding(5) var texture_sampler: sampler;
@group(0) @binding(6) var clip_tex: texture_2d<f32>;
@group(0) @binding(7) var<storage, read> stops: array<vec4<f32>>;

fn frame_h(inst: Instance, p: vec2<f32>) -> vec3<f32> {
	return vec3<f32>(
		dot(inst.transform0.xyz, vec3<f32>(p, 1.0)),
		dot(inst.transform1.xyz, vec3<f32>(p, 1.0)),
		dot(vec3<f32>(inst.transform2.xy, 1.0 - 2.0 * inst.transform2.z), vec3<f32>(p, 1.0)),
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
	} else if (kind < 1.5) {
		let e = 3.0 * inst.params.z + 1.0;
		r = vec4<f32>(r.xy - vec2<f32>(e), r.zw + vec2<f32>(2.0 * e));
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
	out.world = h;
	out.clip_mode = inst.transform1.w;
	out.clip_radii = inst.clip_radii;
	out.transform0 = inst.transform0;
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

fn area_coverage(p: vec2<f32>, rect: vec4<f32>) -> f32 {
	let c = saturate(min(rect.xy + rect.zw, p + vec2<f32>(0.5)) - max(rect.xy, p - vec2<f32>(0.5)));
	return c.x * c.y;
}

const cont_extent: f32 = 1.528665;

struct ContCorner {
	r: f32,
	e: f32,
	eff: f32,
	cx: f32,
	cy: f32,
}

fn cont_corner(r: f32, rh: f32, rv: f32, w: f32, h: f32) -> ContCorner {
	var c: ContCorner;
	c.r = r;
	c.e = cont_extent * r;
	c.cx = saturate((cont_extent - w / max(r + rh, 1e-6)) / (cont_extent - 1.0));
	c.cy = saturate((cont_extent - h / max(r + rv, 1e-6)) / (cont_extent - 1.0));
	c.eff = c.e + (r - c.e) * max(c.cx, c.cy);
	return c;
}

fn cont_dist(c: ContCorner, u: f32, v: f32) -> f32 {
	let a = max(vec2<f32>(0.0), vec2<f32>(1.0) - vec2<f32>(u, v) / c.e);
	let l = length(a);
	let hi = max(a.x, a.y);
	let rho = select(0.0, min(min(a.x, a.y) / hi, 1.0), hi > 0.0);
	let p = (((-0.926054 * rho + 3.15601) * rho - 3.64122) * rho + 1.26803) * rho + 0.268531;
	let f1 = l + 1.0 - 1.0 / (1.0 - rho * rho * min(l, 1.0) * p);
	let f2 = length(max(vec2<f32>(0.0), a * cont_extent - vec2<f32>(cont_extent - 1.0))) * 0.654166 + 0.345834;
	let s = select(-1.0, 1.0, a.y > a.x);
	let w = saturate(0.5 - s + s * rho);
	let fx = f1 + (f2 - f1) * c.cx;
	let fy = f1 + (f2 - f1) * c.cy;
	return min(max(c.eff - u, c.eff - v), 0.0) + c.e * (fx + (fy - fx) * w - 1.0);
}

fn cont_inset(c: ContCorner, v: f32) -> f32 {
	if (v >= c.e || c.r <= 0.0) {
		return 0.0;
	}
	var lo = select(0.0, c.r - sqrt(v * (2.0 * c.r - v)), v < c.r);
	var hi = c.e - sqrt(v * (2.0 * c.e - v));
	var dlo = cont_dist(c, lo, v);
	var dhi = cont_dist(c, hi, v);
	if (dlo <= 0.0) {
		return lo;
	}
	if (dhi >= 0.0) {
		return hi;
	}
	var side = 0;
	for (var n = 0; n < 5; n = n + 1) {
		let m = (lo * dhi - hi * dlo) / (dhi - dlo);
		let dm = cont_dist(c, m, v);
		if (dm > 0.0) {
			lo = m;
			dlo = dm;
			if (side == 1) {
				dhi *= 0.5;
			}
			side = 1;
		} else {
			hi = m;
			dhi = dm;
			if (side == -1) {
				dlo *= 0.5;
			}
			side = -1;
		}
	}
	return (lo * dhi - hi * dlo) / (dhi - dlo);
}

struct ContValue {
	value: f32,
	curved: bool,
}

fn cont_value(p: vec2<f32>, rect: vec4<f32>, c: array<ContCorner, 4>) -> ContValue {
	let d = vec4<f32>(p.x - rect.x, p.y - rect.y, rect.x + rect.z - p.x, rect.y + rect.w - p.y);
	let u = vec4<f32>(d.x, d.z, d.z, d.x);
	let v = vec4<f32>(d.y, d.y, d.w, d.w);
	var value = max(max(-d.x, -d.y), max(-d.z, -d.w));
	var curved = false;
	for (var i = 0; i < 4; i = i + 1) {
		if (c[i].r > 0.0 && u[i] < c[i].e && v[i] < c[i].e) {
			value = max(value, cont_dist(c[i], u[i], v[i]));
			curved = true;
		}
	}
	return ContValue(value, curved);
}

fn cont_coverage(p: vec2<f32>, rect: vec4<f32>, r: vec4<f32>) -> f32 {
	var c: array<ContCorner, 4>;
	c[0] = cont_corner(r.x, r.y, r.w, rect.z, rect.w);
	c[1] = cont_corner(r.y, r.x, r.z, rect.z, rect.w);
	c[2] = cont_corner(r.z, r.w, r.y, rect.z, rect.w);
	c[3] = cont_corner(r.w, r.z, r.x, rect.z, rect.w);
	let d = cont_value(p, rect, c);
	if (!d.curved) {
		return area_coverage(p, rect);
	}
	if (abs(d.value) > 2.0) {
		return select(0.0, 1.0, d.value < 0.0);
	}
	let h = 1.0 / 16.0;
	let dx = cont_value(p + vec2<f32>(h, 0.0), rect, c).value;
	let dy = cont_value(p + vec2<f32>(0.0, h), rect, c).value;
	return saturate(0.5 - d.value * h / max(abs(dx - d.value) + abs(dy - d.value), 1e-6));
}

fn rect_coverage(p: vec2<f32>, rect: vec4<f32>, radii: vec4<f32>) -> f32 {
	if (any(radii < vec4<f32>(0.0))) {
		return cont_coverage(p, rect, -radii);
	}
	let q = p - rect.xy - rect.zw * 0.5;
	let r = select(
		select(radii.z, radii.y, q.y < 0.0),
		select(radii.w, radii.x, q.y < 0.0),
		q.x < 0.0,
	);
	if (r > 0.0) {
		return coverage(sd_round_rect(p, rect, radii));
	}
	return area_coverage(p, rect);
}

fn local_value(p: vec2<f32>, rect: vec4<f32>, r: vec4<f32>) -> f32 {
	if (!any(r < vec4<f32>(0.0))) {
		return sd_round_rect(p, rect, r);
	}
	let a = abs(r);
	var c: array<ContCorner, 4>;
	c[0] = cont_corner(a.x, a.y, a.w, rect.z, rect.w);
	c[1] = cont_corner(a.y, a.x, a.z, rect.z, rect.w);
	c[2] = cont_corner(a.z, a.w, a.y, rect.z, rect.w);
	c[3] = cont_corner(a.w, a.z, a.x, rect.z, rect.w);
	return cont_value(p, rect, c).value;
}

fn local_coverage(
	p: vec2<f32>,
	rect: vec4<f32>,
	radii: vec4<f32>,
	affine: f32,
	dx: vec2<f32>,
	dy: vec2<f32>,
) -> f32 {
	if (affine < 0.5) {
		return rect_coverage(p, rect, radii);
	}
	let reach = max(abs(dx.x) + abs(dx.y), abs(dy.x) + abs(dy.y)) * 2.0;
	let d = local_value(p, rect, radii);
	if (d < -reach) {
		return 1.0;
	}
	if (d > reach) {
		return 0.0;
	}
	var n = 0.0;
	for (var y = 0; y < 4; y = y + 1) {
		for (var x = 0; x < 4; x = x + 1) {
			let q = p
				+ dx * ((f32(x) + 0.5) / 4.0 - 0.5)
				+ dy * ((f32(y) + 0.5) / 4.0 - 0.5);
			if (local_value(q, rect, radii) <= 0.0) {
				n += 1.0;
			}
		}
	}
	return n / 16.0;
}

fn unpremul(c: vec4<f32>) -> vec3<f32> {
	return select(vec3<f32>(0.0), c.rgb / c.a, c.a > 0.0);
}

fn text_coverage(a: f32, c: vec3<f32>, contrast: f32, boost: f32, g: vec4<f32>) -> f32 {
	let k = contrast * saturate(3.0 - 4.0 * dot(c, vec3<f32>(0.30, 0.59, 0.11))) + boost;
	let adjusted = a * (k + 1.0) / (a * k + 1.0);
	let f = dot(c, vec3<f32>(0.25, 0.5, 0.25));
	return saturate(adjusted + adjusted * (1.0 - adjusted) * ((g.x * f + g.y) * adjusted + (g.z * f + g.w)));
}

fn tint(c: vec4<f32>, t: vec4<f32>) -> vec4<f32> {
	return c * vec4<f32>(t.rgb * t.a, t.a);
}

fn premul(c: vec4<f32>) -> vec4<f32> {
	return vec4<f32>(c.rgb * c.a, c.a);
}

fn to_linear3(c: vec3<f32>) -> vec3<f32> {
	return select(c / 12.92, pow((c + vec3<f32>(0.055)) / 1.055, vec3<f32>(2.4)), c > vec3<f32>(0.04045));
}

fn to_srgb3(c: vec3<f32>) -> vec3<f32> {
	return select(c * 12.92, 1.055 * pow(max(c, vec3<f32>(0.0)), vec3<f32>(1.0 / 2.4)) - 0.055, c > vec3<f32>(0.0031308));
}

fn cbrt3(v: vec3<f32>) -> vec3<f32> {
	return sign(v) * pow(abs(v), vec3<f32>(1.0 / 3.0));
}

fn oklab(srgb: vec3<f32>) -> vec3<f32> {
	let c = to_linear3(srgb);
	let lms = cbrt3(vec3<f32>(
		0.4122214708 * c.r + 0.5363325363 * c.g + 0.0514459929 * c.b,
		0.2119034982 * c.r + 0.6806995451 * c.g + 0.1073969566 * c.b,
		0.0883024619 * c.r + 0.2817188376 * c.g + 0.6299787005 * c.b,
	));
	return vec3<f32>(
		0.2104542553 * lms.x + 0.7936177850 * lms.y - 0.0040720468 * lms.z,
		1.9779984951 * lms.x - 2.4285922050 * lms.y + 0.4505937099 * lms.z,
		0.0259040371 * lms.x + 0.7827717662 * lms.y - 0.8086757660 * lms.z,
	);
}

fn from_oklab(lab: vec3<f32>) -> vec3<f32> {
	var lms = vec3<f32>(
		lab.x + 0.3963377774 * lab.y + 0.2158037573 * lab.z,
		lab.x - 0.1055613458 * lab.y - 0.0638541728 * lab.z,
		lab.x - 0.0894841775 * lab.y - 1.2914855480 * lab.z,
	);
	lms = lms * lms * lms;
	return to_srgb3(vec3<f32>(
		4.0767416621 * lms.x - 3.3077115913 * lms.y + 0.2309699292 * lms.z,
		-1.2684380046 * lms.x + 2.6097574011 * lms.y - 0.3413193965 * lms.z,
		-0.0041960863 * lms.x - 0.7034186147 * lms.y + 1.7076147010 * lms.z,
	));
}

fn stop(k: i32) -> vec4<f32> {
	return stops[u32(k)];
}

fn stops_at(c2: vec4<f32>, t: f32, lab: bool) -> vec4<f32> {
	let first = i32(c2.x);
	let n = i32(c2.y);
	if (n <= 0) {
		return vec4<f32>(0.0);
	}
	var a = stop(2 * first);
	var ap = stop(2 * first + 1);
	if (t <= ap.x) {
		return premul(a);
	}
	let last = stop(2 * (first + n - 1));
	let lastp = stop(2 * (first + n - 1) + 1);
	if (t >= lastp.x) {
		return premul(last);
	}
	var b = last;
	var bp = lastp;
	for (var i = 1; i < n - 1; i = i + 1) {
		let c = stop(2 * (first + i));
		let cp = stop(2 * (first + i) + 1);
		if (t < cp.x) {
			b = c;
			bp = cp;
			break;
		}
		a = c;
		ap = cp;
	}
	let f = saturate((t - ap.x) / max(bp.x - ap.x, 1e-6));
	let alpha = mix(a.a, b.a, f);
	if (!lab) {
		return vec4<f32>(mix(a.rgb * a.a, b.rgb * b.a, f), alpha);
	}
	if (alpha <= 0.0) {
		return vec4<f32>(0.0);
	}
	return vec4<f32>(saturate(from_oklab(mix(ap.yzw * a.a, bp.yzw * b.a, f) / alpha)) * alpha, alpha);
}

fn paint(p: vec2<f32>, mode: f32, rect: vec4<f32>, c1: vec4<f32>, c2: vec4<f32>, g: vec4<f32>) -> vec4<f32> {
	if (mode < 0.5) {
		return premul(c1);
	}
	if (mode > 3.5) {
		var t: f32;
		if (mode < 5.5) {
			let d = g.zw - g.xy;
			t = dot(p - g.xy, d) / max(dot(d, d), 0.0001);
		} else if (mode < 7.5) {
			let d = p - g.xy;
			t = (atan2(d.x, -d.y) - g.z) * 0.15915494309189535;
			t -= floor(t);
		} else {
			t = length(p - g.xy) / max(g.z, 0.0001);
		}
		return stops_at(c2, t, (mode > 4.5 && mode < 5.5) || (mode > 6.5 && mode < 7.5) || mode > 8.5);
	}
	if (mode < 2.5) {
		let d = g.zw - g.xy;
		let t = saturate(dot(p - g.xy, d) / max(dot(d, d), 0.0001));
		let a = mix(c1.a, c2.a, t);
		if (mode < 1.5) {
			return vec4<f32>(mix(c1.rgb * c1.a, c2.rgb * c2.a, t), a);
		}
		let lab = mix(oklab(c1.rgb) * c1.a, oklab(c2.rgb) * c2.a, t);
		return select(vec4<f32>(0.0), vec4<f32>(saturate(from_oklab(lab / a)) * a, a), a > 0.0);
	}
	let s = dot(p - rect.xy, g.xy);
	let phase = s - g.w * floor(s / g.w);
	let cov = coverage(min(max(-phase, phase - g.z), g.w - phase));
	return premul(c1) * cov + premul(c2) * (1.0 - cov);
}

fn dash(p: vec2<f32>, rect: vec4<f32>, w: vec4<f32>) -> f32 {
	let q = p - rect.xy;
	let dt = select(1e9, q.y / w.x, w.x > 0.0);
	let dr = select(1e9, (rect.z - q.x) / w.y, w.y > 0.0);
	let db = select(1e9, (rect.w - q.y) / w.z, w.z > 0.0);
	let dl = select(1e9, q.x / w.w, w.w > 0.0);
	var s: f32;
	var len: f32;
	var bw: f32;
	if (dt <= dr && dt <= db && dt <= dl) {
		s = q.x;
		len = rect.z;
		bw = w.x;
	} else if (dr <= db && dr <= dl) {
		s = q.y;
		len = rect.w;
		bw = w.y;
	} else if (db <= dl) {
		s = rect.z - q.x;
		len = rect.z;
		bw = w.z;
	} else {
		s = rect.w - q.y;
		len = rect.w;
		bw = w.w;
	}
	var n = max(1.0, floor((len / (3.0 * bw) + 1.0) * 0.5 + 0.5));
	let period = 6.0 * bw;
	n += select(0.0, 1.0, n * period <= len);
	n -= select(0.0, 1.0, (n - 1.0) * period > len);
	let seg = len / (2.0 * n - 1.0);
	let k = floor(s / seg);
	let f = s - k * seg;
	let edge = min(f, seg - f);
	return coverage(select(edge, -edge, k - 2.0 * floor(k * 0.5) < 0.5));
}

fn erf_approx(x: f32) -> f32 {
	let s = sign(x);
	let a = abs(x);
	var v = 1.0 + (0.278393 + (0.230389 + 0.078108 * (a * a)) * a) * a;
	v *= v;
	return s - s / (v * v);
}

fn gaussian(x: f32, sigma: f32) -> f32 {
	return exp(-(x * x) / (2.0 * sigma * sigma)) / (2.50662827463 * sigma);
}

fn shadow_x(x: f32, y: f32, sigma: f32, corner: f32, half: vec2<f32>, continuous: bool, cc: ContCorner) -> f32 {
	let delta = min(half.y - corner - abs(y), 0.0);
	var curved = half.x - corner + sqrt(max(0.0, corner * corner - delta * delta));
	if (continuous) {
		let v = half.y - abs(y);
		curved = select(half.x, half.x - cont_inset(cc, v), v < cc.e);
	}
	let lo = 0.5 + 0.5 * erf_approx((x - curved) * (sqrt(0.5) / sigma));
	let hi = 0.5 + 0.5 * erf_approx((x + curved) * (sqrt(0.5) / sigma));
	return hi - lo;
}

fn box_shadow(p: vec2<f32>, rect: vec4<f32>, sigma: f32, radii: vec4<f32>) -> f32 {
	let r = abs(radii);
	let corner = max(max(r.x, r.y), max(r.z, r.w));
	let continuous = any(radii < vec4<f32>(0.0)) && corner > 0.0;
	let cc = cont_corner(corner, corner, corner, rect.z, rect.w);
	let half = rect.zw * 0.5;
	let q = p - rect.xy - half;
	let start = clamp(-3.0 * sigma, q.y - half.y, q.y + half.y);
	let to = clamp(3.0 * sigma, q.y - half.y, q.y + half.y);
	let dy = (to - start) / 4.0;
	var y = start + dy * 0.5;
	var sum = 0.0;
	for (var n = 0; n < 4; n = n + 1) {
		sum += shadow_x(q.x, q.y - y, sigma, corner, half, continuous, cc) * gaussian(y, sigma) * dy;
		y += dy;
	}
	return sum;
}

fn sample_local(t: texture_2d<f32>, uv: vec2<f32>, src: vec4<f32>, affine: f32) -> vec4<f32> {
	var q = uv;
	if (affine > 0.5) {
		let dims = vec2<f32>(textureDimensions(t));
		let half_pixel = 0.5 / dims;
		q = clamp(q, src.xy + half_pixel, src.zw - half_pixel);
	}
	return textureSampleLevel(t, texture_sampler, q, 0.0);
}

fn sample_image(t: texture_2d<f32>, uv: vec2<f32>, src: vec4<f32>, pixelated: f32, dtex: vec2<f32>) -> vec4<f32> {
	let size = vec2<f32>(textureDimensions(t));
	let lo = min(src.xy, src.zw);
	let hi = max(src.xy, src.zw);
	var q = uv;
	if (pixelated > 0.5) {
		let tc = q * size - vec2<f32>(0.5);
		let i = floor(tc);
		let f = saturate((tc - i - vec2<f32>(0.5)) / clamp(dtex * size, vec2<f32>(1e-5), vec2<f32>(1.0)) + vec2<f32>(0.5));
		q = (i + f + vec2<f32>(0.5)) / size;
	}
	return textureSampleLevel(t, texture_sampler, clamp(q, lo + 0.5 / size, hi - 0.5 / size), 0.0);
}

fn clip_coverage(in: VertexOut, dx: vec2<f32>, dy: vec2<f32>) -> f32 {
	if (in.clip_mode > 2.5) {
		return 1.0;
	}
	let world = in.world.xy / in.world.z;
	if (in.clip_mode < 0.5) {
		return rect_coverage(world, in.clip, in.clip_radii);
	}
	if (in.clip_mode > 1.5) {
		return local_coverage(in.local, in.clip, vec4<f32>(0.0), in.transform0.w, dx, dy) *
			textureSampleLevel(clip_tex, texture_sampler,
				clamp((world - in.clip_radii.xy) / in.clip_radii.zw, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).a;
	}
	return textureSampleLevel(clip_tex, texture_sampler,
		clamp((world - in.clip.xy) / in.clip.zw, vec2<f32>(0.0), vec2<f32>(1.0)), 0.0).a;
}

@fragment
fn fs(in: VertexOut) -> @location(0) vec4<f32> {
	let dx = dpdx(in.local);
	let dy = dpdy(in.local);
	let dtex = vec2<f32>(fwidth(in.tex.x), fwidth(in.tex.y));
	let kind = in.params.x;
	let clip = clip_coverage(in, dx, dy);
	let glyph_cov = select(1.0, local_coverage(in.local, in.rect, vec4<f32>(0.0), 1.0, dx, dy), in.transform0.w > 0.5 && kind >= 2.5 && kind < 5.5);
	var res: vec4<f32>;
	if (kind < 0.5) {
		let outer = local_coverage(in.local, in.rect, in.radii, in.transform0.w, dx, dy);
		let border_paint = in.params.y > 1.5;
		let pc = paint(in.local, in.params.z, in.rect, in.color, in.color2, in.grad);
		res = select(pc, premul(in.color), border_paint) * outer;
		let bw = in.uv_border;
		if (any(bw > vec4<f32>(0.0))) {
			let ir = vec4<f32>(
				in.rect.xy + vec2<f32>(bw.w, bw.x),
				in.rect.zw - vec2<f32>(bw.y + bw.w, bw.x + bw.z),
			);
			let inner_cov = select(0.0, local_coverage(in.local, ir, in.inner, in.transform0.w, dx, dy), ir.z > 0.0 && ir.w > 0.0);
			var bc = saturate(outer - inner_cov);
			let dashed = in.params.y - select(0.0, 2.0, border_paint);
			if (dashed > 0.5) {
				bc *= dash(in.local, in.rect, bw);
			}
			let b = select(premul(in.border), pc * vec4<f32>(in.border.rgb * in.border.a, in.border.a), border_paint) * bc;
			res = b + res * (1.0 - b.a);
		}
	} else if (kind < 1.5) {
		let sigma = in.params.z;
		if (in.params.y > 0.5) {
			var hole = 0.0;
			if (in.uv_border.z > 0.0 && in.uv_border.w > 0.0) {
				hole = select(
					local_coverage(in.local, in.uv_border, in.inner, in.transform0.w, dx, dy),
					box_shadow(in.local, in.uv_border, sigma, in.inner),
					sigma > 0.0,
				);
			}
			res = premul(in.color) * (local_coverage(in.local, in.rect, in.radii, in.transform0.w, dx, dy) * (1.0 - hole));
		} else {
			var shadow = select(
				local_coverage(in.local, in.rect, in.radii, in.transform0.w, dx, dy),
				box_shadow(in.local, in.rect, sigma, in.radii),
				sigma > 0.0,
			);
			if (in.uv_border.z > 0.0 && in.uv_border.w > 0.0) {
				shadow *= 1.0 - local_coverage(in.local, in.uv_border, in.inner, in.transform0.w, dx, dy);
			}
			res = premul(in.color) * shadow;
		}
	} else if (kind < 2.5) {
		let c = paint(in.local, in.params.z, in.rect, in.color, in.color2, in.grad);
		res = c * text_coverage(
			sample_local(mask_tex, in.tex, in.uv_border, 1.0).r,
			unpremul(c), in.inner.x, in.inner.y, in.radii,
		) * glyph_cov;
	} else if (kind < 3.5) {
		res = sample_local(color_tex, in.tex, in.uv_border, 1.0) * in.color.a * glyph_cov;
	} else if (kind < 4.5) {
		res = sample_image(image_tex, in.tex, in.uv_border, in.params.z, dtex);
		if (in.params.y > 0.5) {
			res = vec4<f32>(vec3<f32>(dot(res.rgb, vec3<f32>(0.2126, 0.7152, 0.0722))), res.a);
		}
		res = tint(res, in.color) * local_coverage(in.local, in.rect, in.radii, in.transform0.w, dx, dy);
	} else {
		let c = paint(in.local, in.params.z, in.rect, in.color, in.color2, in.grad);
		let a = sample_local(color_tex, in.tex, in.uv_border, 1.0).rgb;
		let cov = text_coverage((a.r + a.g + a.b) / 3.0, unpremul(c), in.inner.x, in.inner.y, in.radii);
		res = c * cov * glyph_cov;
	}
	return res * (clip * in.params.w);
}
