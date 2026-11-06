struct ClearColor {
	color: vec4<f32>,
}

@group(0) @binding(0) var<uniform> clear_color: ClearColor;

@vertex
fn clear_vs(@builtin(vertex_index) i: u32) -> @builtin(position) vec4<f32> {
	let corner = vec2<f32>(f32(i & 1u), f32(i >> 1u));
	return vec4<f32>(corner * 2.0 - 1.0, 0.0, 1.0);
}

@fragment
fn clear_fs() -> @location(0) vec4<f32> {
	return clear_color.color;
}
