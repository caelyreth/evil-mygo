@group(0) @binding(0) var src: texture_2d<f32>;
@group(0) @binding(1) var samp: sampler;

@vertex
fn copy_vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	let corner = vec2<f32>(f32(i & 1u), f32(i >> 1u));
	return vec4<f32>(corner * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn copy_fs(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let size = vec2<f32>(textureDimensions(src));
	return textureSampleLevel(src, samp, position.xy / size, 0.0);
}
