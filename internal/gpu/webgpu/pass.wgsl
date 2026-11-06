struct Pass {
	origin: vec2<i32>,
	limit: vec2<i32>,
	shift: vec2<i32>,
	dir: vec2<i32>,
	down: i32,
	radius: i32,
	sigma: f32,
	mode: i32,
}

@group(0) @binding(0) var<storage, read> pass_params: Pass;
@group(0) @binding(1) var src: texture_2d<f32>;

@vertex
fn pass_vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	let corner = vec2<f32>(f32(i & 1u), f32(i >> 1u));
	return vec4<f32>(corner * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn down(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let at0 = pass_params.origin + vec2<i32>(floor(position.xy)) * pass_params.down;
	var c = vec4<f32>(0.0);
	for (var y = 0; y < pass_params.down; y = y + 1) {
		for (var x = 0; x < pass_params.down; x = x + 1) {
			let at = at0 + vec2<i32>(x, y);
			if ((pass_params.mode & 1) != 0 && (at.x > pass_params.limit.x || at.y > pass_params.limit.y)) {
				continue;
			}
			let q = min(at, pass_params.limit) - pass_params.shift;
			c += textureLoad(src, q, 0);
		}
	}
	return c / f32(pass_params.down * pass_params.down);
}

@fragment
fn blur(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let at = vec2<i32>(floor(position.xy));
	var c = vec4<f32>(0.0);
	var sum = 0.0;
	for (var o = -pass_params.radius; o <= pass_params.radius; o = o + 1) {
		let x = f32(o);
		let weight = select(1.0, exp(-(x * x) / (2.0 * pass_params.sigma * pass_params.sigma)), pass_params.sigma > 0.0);
		let q = at + pass_params.dir * o;
		sum += weight;
		if ((pass_params.mode & 1) != 0 &&
			(q.x < 0 || q.y < 0 || q.x > pass_params.limit.x || q.y > pass_params.limit.y)) {
			continue;
		}
		c += textureLoad(src, clamp(q, vec2<i32>(0), pass_params.limit), 0) * weight;
	}
	return c / sum;
}
