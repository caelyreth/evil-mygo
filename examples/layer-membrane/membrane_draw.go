//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"errors"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/egoist/mygo/ui"
)

const membraneShader = `
struct Uniforms {
	viewport: vec4<f32>,
	camera: vec4<f32>,
	lens: vec4<f32>,
}

@group(0) @binding(0) var<uniform> uniforms: Uniforms;
@group(0) @binding(1) var source: texture_2d<f32>;
@group(0) @binding(2) var sourceSampler: sampler;

// Scalar arrays keep the storage stride identical to Go's packed 44-byte vertex.
struct ClothVertexData {
	position: array<f32, 3>,
	normal: array<f32, 3>,
	tangent: array<f32, 3>,
	uv: array<f32, 2>,
}
@group(0) @binding(3) var<storage, read> cloth: array<ClothVertexData>;

struct Basis {
	forward: vec3<f32>,
	right: vec3<f32>,
	up: vec3<f32>,
}

fn cameraBasis() -> Basis {
	let yaw = uniforms.camera.w;
	let pitch = uniforms.lens.x;
	let forward = normalize(vec3<f32>(sin(yaw) * cos(pitch), sin(pitch), -cos(yaw) * cos(pitch)));
	let right = normalize(cross(forward, vec3<f32>(0.0, 1.0, 0.0)));
	return Basis(forward, right, normalize(cross(right, forward)));
}

fn project(p: vec3<f32>) -> vec4<f32> {
	let basis = cameraBasis();
	let relative = p - uniforms.camera.xyz;
	let depth = dot(relative, basis.forward);
	let near = uniforms.lens.z;
	let far = uniforms.lens.w;
	return vec4<f32>(
		dot(relative, basis.right) / (uniforms.viewport.z * uniforms.lens.y),
		dot(relative, basis.up) / uniforms.viewport.z,
		far * (depth - near) / (far - near),
		depth,
	);
}

fn displayColor(linear: vec3<f32>) -> vec3<f32> {
	return pow(clamp(linear, vec3<f32>(0.0), vec3<f32>(1.0)), vec3<f32>(1.0 / 2.2));
}

@vertex
fn environmentVertex(@builtin(vertex_index) index: u32) -> @builtin(position) vec4<f32> {
	var positions = array<vec2<f32>, 3>(
		vec2<f32>(-1.0, -1.0), vec2<f32>(3.0, -1.0), vec2<f32>(-1.0, 3.0),
	);
	return vec4<f32>(positions[index], 1.0, 1.0);
}

fn gridLine(p: vec2<f32>, scale: f32) -> f32 {
	let q = p * scale;
	let footprint = max(fwidth(q), vec2<f32>(0.0001));
	let distance = abs(fract(q + vec2<f32>(0.5)) - vec2<f32>(0.5)) / footprint;
	let coverage = 1.0 - smoothstep(0.45, 1.35, min(distance.x, distance.y));
	return coverage * (1.0 - smoothstep(0.3, 1.0, max(footprint.x, footprint.y)));
}

struct EnvironmentOutput {
	@location(0) color: vec4<f32>,
	@builtin(frag_depth) depth: f32,
}

@fragment
fn environmentFragment(@builtin(position) position: vec4<f32>) -> EnvironmentOutput {
	let screen = position.xy / uniforms.viewport.xy;
	let ndc = vec2<f32>(screen.x * 2.0 - 1.0, 1.0 - screen.y * 2.0);
	let basis = cameraBasis();
	let ray = normalize(basis.forward + basis.right * ndc.x * uniforms.viewport.z * uniforms.lens.y + basis.up * ndc.y * uniforms.viewport.z);
	let halo = pow(max(dot(ray, normalize(vec3<f32>(-0.3, 0.25, -1.0))), 0.0), 5.0);
	var color = mix(vec3<f32>(0.0018, 0.0030, 0.0050), vec3<f32>(0.009, 0.014, 0.022), halo);
	var depth = 1.0;
	let denominator = select(-1.0, 1.0, ray.y >= 0.0) * max(abs(ray.y), 0.000001);
	let t = (uniforms.viewport.w - uniforms.camera.y) / denominator;
	let floor = uniforms.camera.xyz + ray * t;
	let projected = project(floor);
	// Evaluate derivatives before the visibility branch, including at the horizon.
	let minor = gridLine(floor.xz, 4.0);
	let major = gridLine(floor.xz, 1.0);
	let radius = length(floor.xz);
	let fade = exp(-0.045 * radius * radius);
	let pool = exp(-0.18 * radius * radius);
	let ground = vec3<f32>(0.004, 0.006, 0.009) + pool * vec3<f32>(0.004, 0.005, 0.006)
		+ fade * (minor * vec3<f32>(0.009, 0.012, 0.016) + major * vec3<f32>(0.016, 0.022, 0.029));
	if (t > 0.0 && projected.w >= uniforms.lens.z && projected.w < uniforms.lens.w) {
		color = mix(color, ground, exp(-0.012 * t));
		depth = clamp(projected.z / projected.w, 0.0, 1.0);
	}
	let vignette = 1.0 - 0.32 * dot(screen - vec2<f32>(0.5), screen - vec2<f32>(0.5));
	return EnvironmentOutput(vec4<f32>(displayColor(color * vignette), 1.0), depth);
}

struct ClothOutput {
	@builtin(position) position: vec4<f32>,
	@location(0) world: vec3<f32>,
	@location(1) normal: vec3<f32>,
	@location(2) tangent: vec3<f32>,
	@location(3) uv: vec2<f32>,
}

@vertex
fn clothVertex(@builtin(vertex_index) index: u32) -> ClothOutput {
	let input = cloth[index];
	let position = vec3<f32>(input.position[0], input.position[1], input.position[2]);
	let normal = vec3<f32>(input.normal[0], input.normal[1], input.normal[2]);
	let tangent = vec3<f32>(input.tangent[0], input.tangent[1], input.tangent[2]);
	return ClothOutput(project(position), position, normal, tangent, vec2<f32>(input.uv[0], input.uv[1]));
}

@fragment
fn clothFragment(input: ClothOutput, @builtin(front_facing) front: bool) -> @location(0) vec4<f32> {
	// One sheet, two lit sides. The ink shows through the back of the silk.
	let tex = textureSample(source, sourceSampler, input.uv);
	let print = pow(tex.rgb, vec3<f32>(2.2));
	let ivory = vec3<f32>(0.91, 0.87, 0.77);
	var base = mix(ivory, print, select(0.60, 1.0, front));
	let n = normalize(input.normal) * select(-1.0, 1.0, front);
	let tangent = normalize(input.tangent - n * dot(n, input.tangent));
	let bitangent = cross(n, tangent);
	let view = normalize(uniforms.camera.xyz - input.world);
	let key = normalize(vec3<f32>(-0.45, 0.8, 0.65));
	let fill = normalize(vec3<f32>(0.65, 0.25, -0.7));
	let halfSum = key + view;
	let halfVector = halfSum * inverseSqrt(max(dot(halfSum, halfSum), 0.0001));
	let ndl = max(dot(n, key), 0.0);
	let ndv = max(dot(n, view), 0.0);
	let transmitted = max(dot(-n, key), 0.0);
	let nh = max(dot(n, halfVector), 0.0);
	let ht = dot(tangent, halfVector) / 0.34;
	let hb = dot(bitangent, halfVector) / 0.12;
	let satin = exp(-(ht * ht + hb * hb) / max(nh * nh, 0.025)) * ndl;
	let sheen = pow(1.0 - ndv, 3.0) * (0.3 + 0.7 * ndl);
	let threads = input.uv * vec2<f32>(900.0, 600.0);
	let threadVisibility = clamp(vec2<f32>(1.0) - fwidth(threads), vec2<f32>(0.0), vec2<f32>(1.0));
	let weave = sin(threads.x * 6.283185) * sin(threads.y * 6.283185) * threadVisibility.x * threadVisibility.y;
	let border = min(min(input.uv.x, 1.0 - input.uv.x), min(input.uv.y, 1.0 - input.uv.y));
	let hem = 1.0 - smoothstep(0.002, 0.007, border);
	base *= 1.0 + weave * 0.018 - hem * 0.035;
	let illumination = vec3<f32>(0.40, 0.42, 0.46) + ndl * vec3<f32>(0.61, 0.58, 0.51)
		+ max(dot(n, fill), 0.0) * vec3<f32>(0.14, 0.18, 0.24) + transmitted * vec3<f32>(0.16, 0.14, 0.10);
	let color = base * illumination + vec3<f32>(1.0, 0.94, 0.82) * (satin * 0.24 + sheen * 0.16);
	return vec4<f32>(displayColor(color), 1.0);
}
`

type membraneRenderer struct {
	pipeline        *wgpu.RenderPipeline
	environment     *wgpu.RenderPipeline
	layout          *wgpu.BindGroupLayout
	uniforms        *wgpu.Buffer
	vertices        *wgpu.Buffer
	indices         *wgpu.Buffer
	sampler         *wgpu.Sampler
	bind            *wgpu.BindGroup
	environmentBind *wgpu.BindGroup
	sourceView      *wgpu.TextureView
	sourceAt        *wgpu.Texture
	color           *wgpu.Texture
	depth           *wgpu.Texture
	colorView       *wgpu.TextureView
	depthView       *wgpu.TextureView
	width           int
	height          int
	mesh            *clothMesh
	meshVersion     uint64
}

func (m *membraneRenderer) init(device *wgpu.Device) (err error) {
	defer func() {
		if err != nil {
			m.release()
		}
	}()
	module, err := device.CreateShaderModuleWGSL(membraneShader)
	if err != nil {
		return err
	}
	defer module.Release()
	descriptor := wgpu.RenderPipelineDescriptor{
		Label: "Silk flag",
		Vertex: wgpu.VertexState{
			// Pull vertices from storage to avoid v0.5.5's vertex-layout FFI.
			Module: module, EntryPoint: "clothVertex",
		},
		Primitive: wgpu.PrimitiveState{
			// go-webgpu v0.5.5 passes topology through without converting it.
			// Native TriangleList is 4, unlike gputypes' zero-valued default.
			Topology: wgpu.PrimitiveTopology(4), FrontFace: wgpu.FrontFaceCCW, CullMode: wgpu.CullModeNone,
		},
		DepthStencil: &wgpu.DepthStencilState{
			Format: wgpu.TextureFormatDepth24Plus, DepthWriteEnabled: true, DepthCompare: wgpu.CompareFunctionLessEqual,
		},
		Multisample: wgpu.MultisampleState{Count: 4},
		Fragment: &wgpu.FragmentState{
			Module: module, EntryPoint: "clothFragment",
			Targets: []wgpu.ColorTargetState{{Format: wgpu.TextureFormatRGBA8Unorm, WriteMask: wgpu.ColorWriteMaskAll}},
		},
	}
	m.pipeline, err = device.CreateRenderPipeline(&descriptor)
	if err != nil {
		return err
	}
	// Infer layouts from the shaders, as MyGo's renderer does. The pinned
	// go-webgpu version's explicit layout FFI aborts inside wgpu-native.
	m.layout = m.pipeline.GetBindGroupLayout(0)
	if m.layout == nil {
		return errors.New("silk pipeline has no bind group layout")
	}
	descriptor.Label = "Dark grid stage"
	descriptor.Vertex.EntryPoint = "environmentVertex"
	descriptor.Fragment.EntryPoint = "environmentFragment"
	m.environment, err = device.CreateRenderPipeline(&descriptor)
	if err != nil {
		return err
	}
	m.uniforms, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Silk scene uniforms", Usage: wgpu.BufferUsageUniform | wgpu.BufferUsageCopyDst, Size: 48,
	})
	if err != nil {
		return err
	}
	environmentLayout := m.environment.GetBindGroupLayout(0)
	if environmentLayout == nil {
		return errors.New("grid pipeline has no bind group layout")
	}
	defer environmentLayout.Release()
	m.environmentBind, err = device.CreateBindGroupSimple(environmentLayout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, m.uniforms, 0, 48),
	})
	if err != nil {
		return err
	}
	m.vertices, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Shared cloth vertices", Usage: wgpu.BufferUsageStorage | wgpu.BufferUsageCopyDst,
		Size: uint64((clothColumns + 1) * (clothRows + 1) * int(unsafe.Sizeof(clothVertex{}))),
	})
	if err != nil {
		return err
	}
	m.indices, err = device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "Cloth triangles", Usage: wgpu.BufferUsageIndex | wgpu.BufferUsageCopyDst,
		Size: clothColumns * clothRows * 6 * 4,
	})
	if err != nil {
		return err
	}
	m.sampler, err = device.CreateSampler(&wgpu.SamplerDescriptor{
		Label: "Silk print sampler", AddressModeU: wgpu.AddressModeClampToEdge,
		AddressModeV: wgpu.AddressModeClampToEdge, AddressModeW: wgpu.AddressModeClampToEdge,
		MagFilter: wgpu.FilterModeLinear, MinFilter: wgpu.FilterModeLinear,
		MipmapFilter: gputypes.MipmapFilterModeLinear, Anisotropy: 16,
	})
	return err
}

func (m *membraneRenderer) resize(f ui.WebGPUFrame) error {
	if m.color != nil && m.width == f.Width && m.height == f.Height {
		return nil
	}
	color, err := f.Device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "Silk MSAA color", Usage: wgpu.TextureUsageRenderAttachment,
		Size:   wgpu.Extent3D{Width: uint32(f.Width), Height: uint32(f.Height), DepthOrArrayLayers: 1},
		Format: wgpu.TextureFormatRGBA8Unorm, MipLevelCount: 1, SampleCount: 4,
	})
	if err != nil {
		return err
	}
	colorView, err := color.CreateView(nil)
	if err != nil {
		color.Release()
		return err
	}
	depth, err := f.Device.CreateTexture(&wgpu.TextureDescriptor{
		Label: "Silk MSAA depth", Usage: wgpu.TextureUsageRenderAttachment,
		Size:   wgpu.Extent3D{Width: uint32(f.Width), Height: uint32(f.Height), DepthOrArrayLayers: 1},
		Format: wgpu.TextureFormatDepth24Plus, MipLevelCount: 1, SampleCount: 4,
	})
	if err != nil {
		colorView.Release()
		color.Release()
		return err
	}
	depthView, err := depth.CreateView(nil)
	if err != nil {
		depth.Release()
		colorView.Release()
		color.Release()
		return err
	}
	m.releaseTargets()
	m.color, m.colorView, m.depth, m.depthView = color, colorView, depth, depthView
	m.width, m.height = f.Width, f.Height
	return nil
}

func (m *membraneRenderer) draw(f ui.WebGPUFrame, surface *membraneSurface) error {
	if f.Device == nil || f.Encoder == nil || f.Texture == nil || f.SourceTexture == nil || f.Width <= 0 || f.Height <= 0 {
		return nil
	}
	if m.pipeline == nil {
		if err := m.init(f.Device); err != nil {
			return err
		}
	}
	if err := m.resize(f); err != nil {
		return err
	}
	if m.bind == nil || m.sourceAt != f.SourceTexture {
		view, err := f.SourceTexture.CreateView(nil)
		if err != nil {
			return err
		}
		bind, err := f.Device.CreateBindGroupSimple(m.layout, []wgpu.BindGroupEntry{
			wgpu.BufferBindingEntry(0, m.uniforms, 0, 48),
			wgpu.TextureBindingEntry(1, view), wgpu.SamplerBindingEntry(2, m.sampler),
			wgpu.BufferBindingEntry(3, m.vertices, 0, uint64((clothColumns+1)*(clothRows+1)*int(unsafe.Sizeof(clothVertex{})))),
		})
		if err != nil {
			view.Release()
			return err
		}
		if m.bind != nil {
			m.bind.Release()
			m.sourceView.Release()
		}
		m.bind, m.sourceView, m.sourceAt = bind, view, f.SourceTexture
	}
	mesh := surface.geometry()
	if m.mesh != mesh || m.meshVersion != mesh.version {
		if err := f.Device.Queue().WriteBuffer(m.vertices, 0, gpuBytes(mesh.vertices)); err != nil {
			return err
		}
		if m.mesh != mesh {
			if err := f.Device.Queue().WriteBuffer(m.indices, 0, gpuBytes(mesh.indices)); err != nil {
				return err
			}
		}
		m.mesh, m.meshVersion = mesh, mesh.version
	}
	cam := surface.cam
	values := [12]float32{
		float32(f.Width), float32(f.Height), fieldOfView, groundY,
		cam.x, cam.y, cam.z, cam.yaw, cam.pitch, surface.aspect(), nearPlane, farPlane,
	}
	if err := f.Device.Queue().WriteBuffer(m.uniforms, 0, gpuBytes(values[:])); err != nil {
		return err
	}
	view, err := f.Texture.CreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()
	pass, err := beginSilkPass(f.Encoder, m.colorView, view, m.depthView)
	if err != nil {
		return err
	}
	pass.SetPipeline(m.environment)
	pass.SetBindGroup(0, m.environmentBind, nil)
	pass.Draw(3, 1, 0, 0)
	pass.SetPipeline(m.pipeline)
	pass.SetBindGroup(0, m.bind, nil)
	pass.SetIndexBuffer(m.indices, wgpu.IndexFormatUint32, 0, uint64(len(gpuBytes(mesh.indices))))
	pass.DrawIndexed(uint32(len(mesh.indices)), 1, 0, 0, 0)
	pass.End()
	pass.Release()
	return nil
}

func gpuBytes[T any](values []T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(values))), len(values)*int(unsafe.Sizeof(*new(T))))
}

func (m *membraneRenderer) releaseTargets() {
	if m.colorView != nil {
		m.colorView.Release()
		m.depthView.Release()
		m.color.Release()
		m.depth.Release()
	}
	m.color, m.depth, m.colorView, m.depthView = nil, nil, nil, nil
}

func (m *membraneRenderer) release() {
	m.releaseTargets()
	if m.bind != nil {
		m.bind.Release()
	}
	if m.environmentBind != nil {
		m.environmentBind.Release()
	}
	if m.sourceView != nil {
		m.sourceView.Release()
	}
	if m.sampler != nil {
		m.sampler.Release()
	}
	if m.vertices != nil {
		m.vertices.Release()
	}
	if m.indices != nil {
		m.indices.Release()
	}
	if m.uniforms != nil {
		m.uniforms.Release()
	}
	if m.pipeline != nil {
		m.pipeline.Release()
	}
	if m.environment != nil {
		m.environment.Release()
	}
	if m.layout != nil {
		m.layout.Release()
	}
	*m = membraneRenderer{}
}
