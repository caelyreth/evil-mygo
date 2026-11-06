# WebGPU backend

The long-term GPU path is
[`github.com/go-webgpu/webgpu`](https://github.com/go-webgpu/webgpu),
pinned to `v0.5.5`, on top of wgpu-native v29. It calls wgpu-native through
pure Go FFI when built with `-tags nofakecgo`; a normal cgo build avoids
the purego/goffi fake-cgo collision and is the default on macOS.

On macOS wgpu-native selects Metal. The direct Objective-C/Metal renderer
and its public composition API have been retired.

## Runtime

wgpu-native is a runtime library, not a Go dependency. Install the pinned
library for local development:

```sh
go run github.com/go-webgpu/webgpu/cmd/setup@v0.5.5 ./lib
```

The development search order is:

1. `WGPU_NATIVE_PATH`
2. `lib/` next to the executable or above the working directory
3. the operating system's library search path

`/lib/` is intentionally ignored by Git. Distribution packaging must place
the correct `libwgpu_native` beside the application or set
`WGPU_NATIVE_PATH`.

## Current foundation

`internal/gpu/webgpu` now owns:

- runtime discovery before wgpu-native initialization;
- a high-performance adapter and logical device;
- the default queue;
- adapter metadata, including verification that macOS selected Metal;
- a surface over the platform's `CAMetalLayer`;
- format, alpha-mode and present-mode negotiation;
- configure, acquire and present operations;
- a scene renderer driven by MyGo's existing fourteen-float4
  `gpu.Instance` stream: fills, gradients and stops, stripes, continuous
  corners, borders, dashes, outer and inner shadows, projective
  transforms, image batches, grayscale and color glyphs, and cached clip
  masks;
- per-target instance, uniform, atlas and bind-group state, so nested UI
  canvases render before the root pass without replacing the root's
  buffers;
- WebGPU-native canvases and imported textures;
- deterministic release of queue, device, adapter and instance.

The integration test clears a real offscreen texture, copies it to a
readback buffer and verifies the pixels. It skips when wgpu-native is not
installed.

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go test ./internal/gpu/webgpu -count=1 -v'
```

The `nofakecgo` test creates a detached `CAMetalLayer`, configures a real
surface, acquires its texture, clears it, submits and presents:

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go test -tags nofakecgo ./internal/gpu/webgpu -count=1 -v'
```

The renderer tests build normal `scene.Scene` values, encode them through
WebGPU into offscreen textures, read them back and verify cleared
backgrounds, rounded fills, gradients, borders, image batches, grayscale
glyphs, blurred shadows, nested canvases and native WebGPU passes. A UI
integration test also renders a real offscreen `ui.UILayer`, including its
own atlas and nested canvas, through the WebGPU renderer.

`MYGO_PARTIAL=1` enables the native-surface damage path. It keeps the
surface's content in a persistent WebGPU texture, redraws only the
`scene.Differ` damage through a scissored clear, and copies that texture
to the presented surface. This does not depend on a swapchain preserving
its contents. The offscreen `Renderer.RenderTargetPart` primitive is
always available for targets whose contents the application owns. The copy
has a fixed full-surface cost, so the mode remains opt-in; use it when the
damage is small relative to the work saved in a complex frame.

## Native UI

On macOS, a cgo-enabled build uses this WebGPU renderer by default. Without
cgo, the native UI CPU renderer is used unless the app opts into the
cgo-free WebGPU path:

```sh
go run ./examples/counter-native

# The cgo-free WebGPU path, for tests and pure-Go distributions:
CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/counter-native

# A normal cgo build uses WebGPU above.
```

`nofakecgo` is required for the cgo-free WebGPU build because MyGo's macOS
platform package still uses
`ebitengine/purego`, while go-webgpu uses goffi; both provide the fake-cgo
runtime under `CGO_ENABLED=0`. The tag keeps goffi's copy and uses
purego's. A cgo build needs neither tag. Removing the tag from the cgo-free
path is part of the migration, not an application concern.

## WebGPU canvases

`ui.NewWebGPUCanvas` returns an image whose pixels are encoded by an
application callback into a renderer-owned texture:

```go
canvas := ui.NewWebGPUCanvas(960, 540, func(f ui.WebGPUFrame) error {
	// Create pipelines and buffers from f.Device. Encode work into
	// f.Encoder against f.Texture. Do not submit or present.
	return drawScene(f)
})

ui.Box(c).Fill().Children(func() {
	ui.Image(c, canvas.Bitmap()).Fill()
})
```

MyGo clears the target to transparent, invokes `Draw` on the main thread,
and then samples the target in the same encoded frame. The callback may
encode render or compute passes. `WebGPUFrame.Source` is the rendered
native subtree when the canvas is used with `Element.Effect`;
`WebGPUFrame.SourceTexture` is its stable backing texture, so a callback
can cache a view and bind group instead of retaining a frame-scoped view.

`ui.NewWebGPUTexture` publishes an application-owned `*wgpu.Texture` as an
image. MyGo creates only a view and samples it; there is no CPU upload or
readback. The texture and device must belong to the window renderer, and
the texture must be premultiplied RGBA8 in MyGo's row order. Commands
submitted through the device's default queue are ordered with MyGo's
frame. `WebGPUTexture.SetSync` adds explicit `Wait` and `Signal` callbacks
for a producer that is not already ordered with that queue. `Wait` runs
before first sampling; `Signal` runs after all sampling has been encoded.
Neither callback may submit the frame or block the main thread. An owner
replaced by `WebGPUTexture.SetTexture` is retained until the submitted
frame completes, then released.

[`examples/flagscape`](../../examples/flagscape) shows the whole path: a
3D WebGPU scene samples a captured native MyGo face, while WASD,
pointer-look and UV hit-testing remain in the native tree.

[`examples/webgpu-layer`](../../examples/webgpu-layer) shows an
independent `UILayer` used as a WebGPU source, with `PlanarSurface` and
`HostLayer` routing pointer, focus, IME, cursor and accessibility back
into the native layer.

## Effects

`scene.Effect.WGSL` is the WebGPU effect boundary. It defines:

```wgsl
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32>
```

`p` is in device pixels in the effect's local box, and `e.rect`, `e.radii`
and `e.p0` through `e.p4` are the same shape and parameters the Direct3D
and CPU effects receive. The renderer supplies `T0` to
`T3`, `clip_tex`, `mask_tex`, `effect_sampler`, `shader_uniforms` and the
backdrop area fields. The effect returns premultiplied color; MyGo applies
the shape, clip and opacity afterward.

Backdrop-reading effects are supported through a copyable render target,
RGBA8 downsample and separable blur textures. Generated `ui/shader`
libraries emit WGSL through `internal/shaderir/wgsl` and share the packed
shader uniform layout in a byte-addressable `shader_uniforms` storage
buffer.
The basic uniform, bool, derivative, masked-glyph, distortion-effect and
glass-plugin paths have WebGPU tests; the full multi-image shader parity
test is opt-in:

```sh
MYGO_GPU_STRICT=1 CGO_ENABLED=1 go test ./ui/shader
```

The glass plugin now has WGSL shaders and is tested against the CPU
rasterizer on WebGPU.

## Remaining parity work

The direct Metal renderer and Metal-specific public APIs have been
removed. These WebGPU paths remain:

- validating and promoting the opt-in native-surface damage path;
- benchmark parity;
- removal of the `nofakecgo` requirement for fully cgo-free WebGPU builds.

## `purego` build tag

Both `github.com/ebitengine/purego` and `github.com/go-webgpu/goffi`
provide the runtime symbols needed by `CGO_ENABLED=0` FFI. Linking both
without a tag duplicates `_cgo_init`.

goffi's supported solution is its `nofakecgo` tag: purego supplies the
shared runtime while goffi uses those symbols. Any binary that links MyGo's
platform packages together with the WebGPU backend must therefore be built
with:

```sh
-tags nofakecgo
```

The standalone WebGPU device test does not import purego and needs no tag.
The surface-owning and native UI integration tests do.

## Migration guardrails

- Platform integration must pass `nofakecgo` until `purego` is removed
  from the final native stack or upstream provides automatic arbitration.
- Application code must use WebGPU texture/buffer handles. It must not
  require a raw `MTLTexture`.
- WebGPU's portable texture model replaces the previous same-Metal-device
  import contract. Cross-process or cross-API shared handles remain an
  explicit follow-up, not an assumed zero-copy path.

## Upstream caveat

go-webgpu v0.5.5's `MappedRange.Bytes` calls
`wgpuBufferGetMapState`, which wgpu-native v29 leaves unimplemented and
aborts the process. MyGo uses `Buffer.GetMappedRange` after a successful
`Map`; this must remain behind a framework helper so callers cannot hit the
broken path accidentally.

`PrimitiveTopology` is also passed through without the
gputypes-to-native conversion applied to other enums. Native
`TriangleStrip` is value 5; using the gputypes value selects
`TriangleList` and draws only half of each quad. The renderer carries this
workaround with a regression test until a fixed binding release is pinned.
