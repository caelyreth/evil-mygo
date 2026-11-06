# WebGPU backend

The long-term GPU path is
[`github.com/go-webgpu/webgpu`](https://github.com/go-webgpu/webgpu),
pinned to `v0.5.5`, on top of wgpu-native v29. It calls wgpu-native through
pure Go FFI, so the framework remains `CGO_ENABLED=0`.

On macOS wgpu-native selects Metal. MyGo is moving from direct
Objective-C/Metal drawing to WebGPU while keeping the same generated UI
scenes and native input/accessibility contracts.

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

## Native UI

The WebGPU backend is currently opt-in while effects and custom shader
libraries finish moving from the direct Metal path:

```sh
CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/counter-native
```

`nofakecgo` is required because MyGo's macOS platform package still uses
`ebitengine/purego`, while go-webgpu uses goffi; both provide the fake-cgo
runtime under `CGO_ENABLED=0`. The tag keeps goffi's copy and uses
purego's. Native UI apps remain cgo-free. Removing that build-tag
requirement is part of the migration, not an application concern.

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
native subtree when the canvas is used with `Element.Effect`; it is absent
otherwise.

`ui.NewWebGPUTexture` publishes an application-owned `*wgpu.Texture` as an
image. MyGo creates only a view and samples it; there is no CPU upload or
readback. The texture and device must belong to the window renderer, and
the texture must be premultiplied RGBA8 in MyGo's row order. Commands
submitted through the device's default queue are ordered with MyGo's
frame.

## Remaining parity work

The direct Metal renderer remains temporarily as the default until these
WebGPU paths are complete:

- `scene.Effect` pipelines and backdrop textures;
- the `ui/shader`-generated shader libraries and the glass plugin;
- holes, wide-gamut presentation and damage-aware presentation;
- benchmark parity and the purego/goffi build-tag removal;
- migration of the Metal examples and public names to WebGPU.

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
The surface-owning test and the eventual renderer integration do.

## Migration guardrails

- The direct Metal renderer stays temporarily as a reference, but new
  public GPU work must target WebGPU.
- Platform integration must pass `nofakecgo` until `purego` is removed
  from the final native stack or upstream provides automatic arbitration.
- The Metal implementation is removed only after WebGPU parity for the
  scene renderer, offscreen canvases, custom passes, imported surfaces,
  synchronization, wide color, effects and benchmarks.
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
