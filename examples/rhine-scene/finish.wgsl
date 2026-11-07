struct Globals {
	viewProjection: mat4x4<f32>, lightProjection: mat4x4<f32>,
	camera: vec4<f32>, right: vec4<f32>, up: vec4<f32>, forward: vec4<f32>,
	lens: vec4<f32>, size: vec4<f32>, light: vec4<f32>, underlight: vec4<f32>,
	pages: vec4<f32>,
}
@group(0) @binding(0) var<uniform> globals: Globals;
@group(0) @binding(1) var color: texture_2d<f32>;
@group(0) @binding(2) var depth: texture_depth_multisampled_2d;
@group(0) @binding(3) var linearSampler: sampler;

@vertex
fn vertexMain(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	return vec4<f32>(p[index], 0.0, 1.0);
}

fn linearDepth(pixel: vec2<i32>) -> f32 {
	let p = clamp(pixel, vec2<i32>(0), vec2<i32>(globals.size.xy) - vec2<i32>(1));
	let d = textureLoad(depth, p, 0);
	let near = globals.lens.z;
	let far = globals.lens.w;
	return near * far / (far - d * (far - near));
}

fn aces(x: vec3<f32>) -> vec3<f32> {
	return clamp((x * (2.51 * x + vec3<f32>(0.03))) / (x * (2.43 * x + vec3<f32>(0.59)) + vec3<f32>(0.14)), vec3<f32>(0.0), vec3<f32>(1.0));
}

@fragment
fn fragmentMain(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let uv = position.xy / globals.size.xy;
	let distance = linearDepth(vec2<i32>(position.xy));
	// A restrained depth-aware lens blur; foreground details stay legible.
	let blur = min(4.0, abs(distance - globals.size.z) * 0.22);
	var sum = textureSampleLevel(color, linearSampler, uv, 0.0).rgb;
	var weight = 1.0;
	for (var i = 0; i < 12; i++) {
		let angle = f32(i) * 2.399963;
		let offset = vec2<f32>(cos(angle), sin(angle)) * sqrt((f32(i) + 0.5) / 12.0) * blur;
		let otherDepth = linearDepth(vec2<i32>(position.xy + offset));
		let w = exp(-abs(otherDepth - distance) * 0.7);
		sum += textureSampleLevel(color, linearSampler, uv + offset / globals.size.xy, 0.0).rgb * w;
		weight += w;
	}
	var shaded = sum / weight;
	// A soft edge haze keeps the atmosphere inside the scene's HDR pipeline.
	let edge = pow(abs(uv.x - 0.5) * 2.0, 5.0) * 0.12 + pow(abs(uv.y - 0.48) * 1.8, 6.0) * 0.32;
	let atmosphere = mix(vec3<f32>(1.13, 1.075, 1.025), vec3<f32>(0.009, 0.015, 0.027), clamp(globals.light.w, 0.0, 1.0));
	shaded = mix(shaded, atmosphere, clamp(edge, 0.0, 0.40));
	let mapped = aces(shaded * 1.05);
	let srgb = select(mapped * 12.92, 1.055 * pow(mapped, vec3<f32>(1.0 / 2.4)) - vec3<f32>(0.055), mapped > vec3<f32>(0.0031308));
	return vec4<f32>(srgb, 1.0);
}
