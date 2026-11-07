struct Globals {
	viewProjection: mat4x4<f32>,
	lightProjection: mat4x4<f32>,
	camera: vec4<f32>,
	right: vec4<f32>,
	up: vec4<f32>,
	forward: vec4<f32>,
	lens: vec4<f32>,
	size: vec4<f32>,
	light: vec4<f32>,
	underlight: vec4<f32>,
	pages: vec4<f32>,
}
struct Vertex { position: vec4<f32>, normal: vec4<f32>, uv: vec4<f32> }
struct Instance { position: vec4<f32>, rotation: vec4<f32> }
struct Material { color: vec4<f32>, optics: vec4<f32> }

@group(0) @binding(0) var<uniform> globals: Globals;
@group(0) @binding(1) var<storage, read> vertices: array<Vertex>;
@group(0) @binding(2) var<storage, read> instances: array<Instance>;
@group(0) @binding(3) var<storage, read> materials: array<Material>;
@group(0) @binding(4) var shadowMap: texture_depth_2d;
@group(0) @binding(5) var opaqueScene: texture_2d<f32>;
@group(0) @binding(6) var linearSampler: sampler;
@group(0) @binding(7) var opticalLayers: texture_2d<f32>;
@group(0) @binding(8) var dossierPage: texture_2d<f32>;

const pi = 3.14159265;
const underlightRadius = 0.078;

fn nightMode() -> f32 { return clamp(globals.light.w, 0.0, 1.0); }

fn backgroundColor() -> vec3<f32> {
	return mix(vec3<f32>(1.13, 1.075, 1.025), vec3<f32>(0.009, 0.015, 0.027), nightMode());
}

fn rotate(p: vec3<f32>, rotation: vec4<f32>) -> vec3<f32> {
	return vec3<f32>(p.x, p.y * rotation.y - p.z * rotation.x, p.y * rotation.x + p.z * rotation.y);
}

struct MeshOutput {
	@builtin(position) position: vec4<f32>,
	@location(0) world: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) local: vec3<f32>,
	@location(3) @interpolate(flat) material: u32,
	@location(4) @interpolate(flat) detailed: f32,
	@location(5) @interpolate(flat) fileID: u32,
}

@vertex
fn meshVertex(@builtin(vertex_index) index: u32, @builtin(instance_index) instanceIndex: u32) -> MeshOutput {
	let vertex = vertices[index];
	let instance = instances[instanceIndex];
	let world = rotate(vertex.position.xyz, instance.rotation) + instance.position.xyz;
	return MeshOutput(globals.viewProjection * vec4<f32>(world, 1.0), world,
		rotate(vertex.normal.xyz, instance.rotation), vertex.position.xyz, u32(vertex.normal.w), instance.position.w, u32(instance.rotation.z));
}

fn decodeSRGB(color: vec3<f32>) -> vec3<f32> {
	return select(color / 12.92, pow((color + vec3<f32>(0.055)) / 1.055, vec3<f32>(2.4)), color > vec3<f32>(0.04045));
}

fn indexTexture(local: vec3<f32>, fileID: u32) -> vec3<f32> {
	// Procedural pigment is attached to the physical inlay, not the UI or
	// world-space position. Navigation, extraction and lighting cannot reseed it.
	var hash = fileID;
	hash = (hash ^ (hash >> 16u)) * 0x7feb352du;
	hash = (hash ^ (hash >> 15u)) * 0x846ca68bu;
	hash = hash ^ (hash >> 16u);
	let palette = array<vec3<f32>, 4>(vec3<f32>(0.863, 0.659, 0.329), vec3<f32>(0.247, 0.357, 0.294),
		vec3<f32>(0.549, 0.235, 0.220), vec3<f32>(0.29, 0.42, 0.53));
	let grain = 0.98 + 0.02 * sin(local.x * 900.0) * sin(local.y * 700.0);
	return decodeSRGB(palette[hash & 3u]) * grain;
}

@vertex
fn shadowVertex(@builtin(vertex_index) index: u32, @builtin(instance_index) instanceIndex: u32) -> @builtin(position) vec4<f32> {
	let instance = instances[instanceIndex];
	let world = rotate(vertices[index].position.xyz, instance.rotation) + instance.position.xyz;
	return globals.lightProjection * vec4<f32>(world, 1.0);
}

fn shadowAt(world: vec3<f32>, n: vec3<f32>) -> f32 {
	let projected = globals.lightProjection * vec4<f32>(world + n * 0.012, 1.0);
	let uv = projected.xy / projected.w * vec2<f32>(0.5, -0.5) + vec2<f32>(0.5);
	let depth = projected.z / projected.w;
	if (any(uv < vec2<f32>(0.0)) || any(uv > vec2<f32>(1.0)) || depth < 0.0 || depth >= 1.0) { return 1.0; }
	let dims = vec2<i32>(textureDimensions(shadowMap));
	let center = vec2<i32>(uv * vec2<f32>(dims));
	var sum = 0.0;
	for (var y = -2; y <= 2; y++) {
		for (var x = -2; x <= 2; x++) {
			let pixel = clamp(center + vec2<i32>(x, y) * 2, vec2<i32>(0), dims - vec2<i32>(1));
			sum += select(0.0, 1.0, depth - 0.00025 <= textureLoad(shadowMap, pixel, 0));
		}
	}
	let edge = min(min(uv.x, 1.0 - uv.x), min(uv.y, 1.0 - uv.y));
	return mix(1.0, sum / 25.0, smoothstep(0.0, 0.08, edge));
}

fn roomReflection(direction: vec3<f32>, roughness: f32) -> vec3<f32> {
	let sky = mix(vec3<f32>(0.35, 0.30, 0.25), vec3<f32>(1.12, 1.10, 1.07), smoothstep(-0.65, 0.7, direction.y));
	let panel1 = pow(max(dot(direction, normalize(vec3<f32>(-0.4, 0.65, 0.7))), 0.0), mix(110.0, 4.0, roughness));
	let panel2 = pow(max(dot(direction, normalize(vec3<f32>(0.8, 0.3, -0.4))), 0.0), mix(65.0, 3.0, roughness));
	let day = sky + vec3<f32>(2.5, 2.35, 2.1) * panel1 + vec3<f32>(1.25) * panel2;
	let night = vec3<f32>(0.012, 0.021, 0.039) + vec3<f32>(0.22, 0.29, 0.40) * panel1 + vec3<f32>(0.08, 0.12, 0.19) * panel2;
	return mix(day, night, nightMode());
}

fn brdf(base: vec3<f32>, metalness: f32, roughness: f32, n: vec3<f32>, v: vec3<f32>, l: vec3<f32>) -> vec3<f32> {
	let h = normalize(v + l);
	let nl = max(dot(n, l), 0.0);
	let nv = max(dot(n, v), 0.001);
	let nh = max(dot(n, h), 0.0);
	let vh = max(dot(v, h), 0.0);
	let a = max(roughness * roughness, 0.015);
	let a2 = a * a;
	let d = a2 / max(pi * pow(nh * nh * (a2 - 1.0) + 1.0, 2.0), 0.00001);
	let k = (roughness + 1.0) * (roughness + 1.0) / 8.0;
	let visibility = nv / (nv * (1.0 - k) + k) * nl / (nl * (1.0 - k) + k);
	let f0 = mix(vec3<f32>(0.04), base, metalness);
	let fresnel = f0 + (vec3<f32>(1.0) - f0) * pow(1.0 - vh, 5.0);
	let specular = d * visibility * fresnel / max(4.0 * nv * nl, 0.001);
	return ((vec3<f32>(1.0) - fresnel) * base * (1.0 - metalness) / pi + specular) * nl;
}

fn underlightCenter() -> vec3<f32> {
	// Put the strip in the narrow gap ahead of the selected laminate.
	return globals.underlight.xyz + vec3<f32>(0.0, 0.0, 0.31);
}

fn underlightPoint(world: vec3<f32>) -> vec3<f32> {
	let center = underlightCenter();
	return center + vec3<f32>(clamp(world.x - center.x, -2.15, 2.15), 0.0, 0.0);
}

fn underlightAt(world: vec3<f32>) -> vec3<f32> {
	let delta = underlightPoint(world) - world;
	let distanceSquared = dot(delta, delta);
	// Amber provides local contrast against the high-key ivory studio lighting.
	let reach = max(1.0 - distanceSquared / 49.0, 0.0);
	let falloff = reach * reach / (distanceSquared + 1.2);
	let pigment = mix(vec3<f32>(1.0, 0.28, 0.025), vec3<f32>(0.64, 0.27, 0.085), nightMode());
	return pigment * globals.underlight.w * falloff;
}

fn underlightDirection(world: vec3<f32>) -> vec3<f32> {
	let delta = underlightPoint(world) - world;
	return delta * inverseSqrt(max(dot(delta, delta), 0.0001));
}

fn underlightHit(origin: vec3<f32>, ray: vec3<f32>) -> f32 {
	let center = underlightCenter();
	// Rebase near the lamp to keep the thin-tube intersection accurate even
	// with the archive camera 140 units away.
	let advance = max(dot(center - origin, ray) - 3.0, 0.0);
	let start = origin + ray * advance;
	let a = center - vec3<f32>(2.15, 0.0, 0.0);
	let b = center + vec3<f32>(2.15, 0.0, 0.0);
	let axis = b - a;
	let offset = start - a;
	let aa = dot(axis, axis);
	let ar = dot(axis, ray);
	let ao = dot(axis, offset);
	let ro = dot(ray, offset);
	let qa = aa - ar * ar;
	let qb = aa * ro - ao * ar;
	let qc = aa * dot(offset, offset) - ao * ao - underlightRadius * underlightRadius * aa;
	let discriminant = qb * qb - qa * qc;
	var hit = 1e6;
	if (qa > 0.00001 && discriminant >= 0.0) {
		let t = (-qb - sqrt(discriminant)) / qa;
		let along = ao + t * ar;
		if (t > 0.0 && along >= 0.0 && along <= aa) { hit = t; }
	}
	for (var i = 0; i < 2; i++) {
		let end = select(a, b, i == 1);
		let delta = start - end;
		let qbCap = dot(ray, delta);
		let d = qbCap * qbCap - dot(delta, delta) + underlightRadius * underlightRadius;
		if (d >= 0.0) {
			let t = -qbCap - sqrt(d);
			if (t > 0.0) { hit = min(hit, t); }
		}
	}
	return hit + advance;
}

fn fogColor(color: vec3<f32>, world: vec3<f32>) -> vec3<f32> {
	let depth = dot(world - globals.camera.xyz, globals.forward.xyz);
	let fog = smoothstep(globals.size.w + 1.0, globals.size.w + 22.0, depth);
	return mix(color, backgroundColor(), fog);
}

fn physicalSurface(input: MeshOutput, front: bool) -> vec3<f32> {
	let material = materials[input.material];
	var base = material.color.rgb;
	if (material.optics.w == 4.0) { base = indexTexture(input.local, input.fileID); }
	let roughness = material.color.w;
	let metalness = material.optics.x;
	let n = normalize(input.normal) * select(-1.0, 1.0, front);
	let v = normalize(globals.camera.xyz - input.world);
	let key = normalize(globals.light.xyz);
	let fill = normalize(vec3<f32>(7.0, 8.0, -10.0));
	let shadow = shadowAt(input.world, n);
	if (material.optics.w == 2.0) {
		// Pale cut edges distinguish the content slab from its broad faces.
		let cutEdge = 1.0 - abs(normalize(input.normal).z);
		base = mix(base, vec3<f32>(0.96, 0.91, 0.82), cutEdge * 0.75);
	}
	let hemisphere = mix(vec3<f32>(0.39, 0.32, 0.26), vec3<f32>(0.83, 0.80, 0.75), n.y * 0.5 + 0.5);
	let f0 = mix(vec3<f32>(0.04), base, metalness);
	let reflected = roomReflection(reflect(-v, n), roughness);
	var color = base * hemisphere * (1.0 - metalness) * mix(0.72, 0.055, nightMode());
	let keyColor = mix(vec3<f32>(3.4, 3.15, 2.8), vec3<f32>(0.35, 0.46, 0.65), nightMode());
	color += brdf(base, metalness, roughness, n, v, key) * keyColor * mix(0.22, 1.0, shadow);
	color += brdf(base, metalness, roughness, n, v, fill) * mix(1.1, 0.12, nightMode());
	color += reflected * f0 * (0.28 + 0.72 * (1.0 - roughness));
	let lowerBody = 1.0 - smoothstep(0.4, 3.2, input.local.y);
	color += brdf(base, metalness, roughness, n, v, underlightDirection(input.world)) * underlightAt(input.world) * lowerBody;
	if (material.optics.w == 4.0) { color += base * mix(0.04, 0.13, nightMode()); }
	return color;
}

@fragment
fn opaqueFragment(input: MeshOutput, @builtin(front_facing) front: bool) -> @location(0) vec4<f32> {
	// The HDR capture's alpha stores opaque view depth for refraction rejection.
	let depth = dot(input.world - globals.camera.xyz, globals.forward.xyz);
	return vec4<f32>(fogColor(physicalSurface(input, front), input.world), depth);
}

@fragment
fn dossierFragment(input: MeshOutput, @builtin(front_facing) front: bool) -> @location(0) vec4<f32> {
	// The two native layers occupy separate atlas halves and separate physical
	// core faces. Reverse U on the back, so text is not mirrored through the core.
	if (!front || abs(abs(input.local.z) - 0.07) > 0.00001) { discard; }
	let reverse = input.local.z < 0.0;
	let u = select(input.local.x / 5.0 + 0.5, 0.5 - input.local.x / 5.0, reverse);
	let atlasOffset = select(0.0, 0.5, reverse);
	let inset = 0.5 / vec2<f32>(textureDimensions(dossierPage));
	let uv = clamp(vec2<f32>(atlasOffset + u * 0.5, 1.0 - input.local.y / 3.7),
		vec2<f32>(atlasOffset, 0.0) + inset, vec2<f32>(atlasOffset + 0.5, 1.0) - inset);
	let page = textureSampleLevel(dossierPage, linearSampler, uv, 0.0);
	// Native layer opacity/blur is premultiplied. Unmultiply before decoding
	// sRGB, then blend in linear light over the unchanged physical core.
	if (page.a < 0.0001) { discard; }
	let ink = clamp(page.rgb / page.a, vec3<f32>(0.0), vec3<f32>(1.0));
	let linear = decodeSRGB(ink);
	let n = normalize(input.normal);
	// The native dossier stays readable as a softly illuminated lab display.
	let illumination = (mix(0.82, 0.75, nightMode()) + 0.22 * max(dot(n, normalize(globals.light.xyz)), 0.0)) * mix(0.78, 1.0, shadowAt(input.world, n));
	var color = linear * illumination;
	color += linear * underlightAt(input.world) * 0.012 * (1.0 - smoothstep(0.4, 3.2, input.local.y));
	// Fade the completed page once, in linear light. Native subtree opacity
	// compounds through overlapping panels and produces a jump at either end.
	color = mix(physicalSurface(input, front), color, page.a * globals.pages.x);
	let depth = dot(input.world - globals.camera.xyz, globals.forward.xyz);
	return vec4<f32>(fogColor(color, input.world), depth);
}

@fragment
fn glassFragment(input: MeshOutput, @builtin(front_facing) front: bool) -> @location(0) vec4<f32> {
	// Each bonded outer slab contributes its outward visible face once.
	if (!front) { discard; }
	let material = materials[input.material];
	let n = normalize(input.normal);
	let v = normalize(globals.camera.xyz - input.world);
	let screen = input.position.xy / globals.size.xy;
	let depth = max(dot(input.world - globals.camera.xyz, globals.forward.xyz), globals.lens.z);
	let nv = clamp(dot(n, v), 0.0, 1.0);
	let roughness = mix(material.color.w, 0.13, input.detailed);
	let path = material.optics.z / max(nv, 0.13);
	let pixelsPerWorld = globals.size.y / (2.0 * globals.lens.x * depth);
	let ior = 1.46;
	let bent = refract(-v, n, 1.0 / ior);
	let displacement = (bent / max(abs(dot(bent, n)), 0.15) + v / max(nv, 0.15)) * material.optics.z;
	let projected = vec2<f32>(dot(displacement, globals.right.xyz), -dot(displacement, globals.up.xyz));
	// Object-space etching stays attached to the sheet while it moves.
	let etch = vec2<f32>(sin(input.local.x * 29.0 + input.local.y * 17.0), sin(input.local.y * 31.0 - input.local.x * 13.0));
	let refractedUV = screen + (projected + etch * 0.0015) * pixelsPerWorld / globals.size.xy;
	let radius = clamp(roughness * 0.08 * pixelsPerWorld * sqrt(path / material.optics.z), 0.5, 9.0);
	var behind = textureSampleLevel(opaqueScene, linearSampler, screen, 0.0).rgb * 0.22;
	var weight = 0.22;
	for (var i = 0; i < 12; i++) {
		let angle = f32(i) * 2.399963;
		let offset = vec2<f32>(cos(angle), sin(angle)) * sqrt((f32(i) + 0.5) / 12.0) * radius / globals.size.xy;
		let tap = textureSampleLevel(opaqueScene, linearSampler, refractedUV + offset, 0.0);
		// Never refract a nearer file into a cover that lies behind it.
		let w = (0.78 / 12.0) * smoothstep(depth - 0.015, depth + 0.025, tap.a);
		behind += tap.rgb * w;
		weight += w;
	}
	behind /= weight;
	let f0 = pow((ior - 1.0) / (ior + 1.0), 2.0);
	let fresnel = f0 + (1.0 - f0) * pow(1.0 - nv, 5.0);
	let reflected = roomReflection(reflect(-v, n), roughness);
	let absorption = exp(-vec3<f32>(0.07, 0.13, 0.24) * path * mix(1.0, 0.45, input.detailed));
	let edgeDistance = min(2.5 - abs(input.local.x), min(input.local.y, 3.7 - input.local.y));
	let rim = 1.0 - smoothstep(0.003, 0.045, edgeDistance);
	let key = normalize(globals.light.xyz);
	let rimLight = (0.12 + 0.20 * max(dot(n, key), 0.0)) * mix(1.0, 0.18, nightMode());
	let scatter = (0.025 + 0.055 * max(dot(-n, key), 0.0)) * roughness * mix(1.0, 0.12, nightMode());
	var color = mix(behind * absorption * material.color.rgb, reflected, fresnel);
	color += vec3<f32>(1.04, 0.99, 0.90) * scatter;
	color += vec3<f32>(1.5, 1.4, 1.2) * rim * rimLight;
	let lowerBody = 1.0 - smoothstep(0.3, 3.25, input.local.y);
	let lampDirection = underlightDirection(input.world);
	let lampNormal = n * select(-1.0, 1.0, dot(n, lampDirection) >= 0.0);
	let incoming = refract(-lampDirection, lampNormal, 1.0 / ior);
	// Integrate the moving lamp through the actual polymer thickness along
	// the refracted view ray; the opaque lit insert is also in the blur capture.
	var volumeLight = vec3<f32>(0.0);
	for (var i = 0; i < 4; i++) {
		let travel = path * (f32(i) + 0.5) / 4.0;
		let point = input.world + bent * travel;
		volumeLight += underlightAt(point) * exp(-vec3<f32>(0.14, 0.24, 0.46) * travel) * 0.25;
	}
	let phaseCos = clamp(dot(incoming, -bent), -1.0, 1.0);
	let anisotropy = 0.35;
	let phase = (1.0 - anisotropy * anisotropy) / pow(max(1.0 + anisotropy * anisotropy - 2.0 * anisotropy * phaseCos, 0.1), 1.5);
	let volumeScatter = 1.0 - exp(-path * (1.2 + roughness * 3.5));
	color += volumeLight * phase * volumeScatter * (0.80 + rim * 1.15) * lowerBody;
	color += brdf(material.color.rgb, 0.0, roughness, n, v, lampDirection) * underlightAt(input.world) * 0.60 * lowerBody;
	// Optical-depth accumulation is commutative: neither a second cover nor a
	// different mesh order erases the first cover's contribution.
	// Higher transmission must reduce the cover's contribution, not make the
	// accumulated frost denser. Preserve stronger reflections at grazing edges.
	let opacity = clamp(1.0 - material.optics.y + fresnel * 0.7 + roughness * 0.45 + rim * 0.16, 0.025, 0.85);
	let density = -log(1.0 - opacity);
	return vec4<f32>(fogColor(color, input.world) * density, density);
}

@vertex
fn fullscreenVertex(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
	var p = array<vec2<f32>, 3>(vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0));
	return vec4<f32>(p[index], 1.0, 1.0);
}

@fragment
fn opticalCompositeFragment(@builtin(position) position: vec4<f32>) -> @location(0) vec4<f32> {
	let uv = position.xy / globals.size.xy;
	let opaque = textureSampleLevel(opaqueScene, linearSampler, uv, 0.0).rgb;
	let layers = textureSampleLevel(opticalLayers, linearSampler, uv, 0.0);
	let coverage = 1.0 - exp(-layers.a);
	let plastic = layers.rgb / max(layers.a, 0.00001);
	return vec4<f32>(mix(opaque, plastic, coverage), 1.0);
}

struct GroundOutput { @location(0) color: vec4<f32>, @builtin(frag_depth) depth: f32 }

@fragment
fn groundFragment(@builtin(position) position: vec4<f32>) -> GroundOutput {
	let uv = position.xy / globals.size.xy;
	let ndc = vec2<f32>(uv.x * 2.0 - 1.0, 1.0 - uv.y * 2.0);
	let ray = normalize(globals.forward.xyz + globals.right.xyz * ndc.x * globals.lens.x * globals.lens.y + globals.up.xyz * ndc.y * globals.lens.x);
	var color = backgroundColor();
	var depth = 1.0;
	var viewDepth = globals.lens.w;
	if (ray.y < -0.0001) {
		let t = (-4.63 - globals.camera.y) / ray.y;
		let world = globals.camera.xyz + ray * t;
		let projected = globals.viewProjection * vec4<f32>(world, 1.0);
		if (t > 0.0 && projected.w > globals.lens.z && projected.w < globals.lens.w) {
			let shadow = shadowAt(world, vec3<f32>(0.0, 1.0, 0.0));
			var floorColor = mix(vec3<f32>(0.95, 0.84, 0.72), vec3<f32>(0.017, 0.025, 0.039), nightMode()) * mix(0.68, 1.0, shadow);
			floorColor += underlightAt(world) * 0.16;
			color = fogColor(floorColor, world);
			depth = projected.z / projected.w;
			viewDepth = projected.w;
		}
	}
	// Actual luminous geometry goes into the opaque HDR/depth capture, so files
	// occlude it normally and the plastic can blur and refract the visible strip.
	if (globals.underlight.w > 0.0) {
		let lampDistance = underlightHit(globals.camera.xyz, ray);
		let world = globals.camera.xyz + ray * lampDistance;
		let projected = globals.viewProjection * vec4<f32>(world, 1.0);
		if (projected.w > globals.lens.z && projected.w < min(viewDepth, globals.lens.w)) {
			let strength = clamp(globals.underlight.w / 144.0, 0.0, 1.0);
			let emitter = mix(vec3<f32>(28.0, 2.4, 0.02), vec3<f32>(19.0, 3.25, 0.52), nightMode());
			color = fogColor(emitter * strength, world);
			depth = projected.z / projected.w;
			viewDepth = projected.w;
		}
	}
	return GroundOutput(vec4<f32>(color, viewDepth), depth);
}
