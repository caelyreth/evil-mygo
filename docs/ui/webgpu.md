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
- the first WebGPU scene renderer stage: solid rounded fills and projective
  transforms from MyGo's existing `gpu.Instance` stream;
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

The renderer test builds a normal `scene.Scene`, encodes it through WebGPU
into an offscreen texture, reads it back and verifies both the cleared
background and the rounded fill. Unsupported scene features return an
explicit error until their paths are ported.

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
