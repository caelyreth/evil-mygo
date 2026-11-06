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
- deterministic release of queue, device, adapter and instance.

The integration test clears a real offscreen texture, copies it to a
readback buffer and verifies the pixels. It skips when wgpu-native is not
installed.

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go test ./internal/gpu/webgpu -count=1 -v'
```

## Migration guardrails

- The direct Metal renderer stays temporarily as a reference, but new
  public GPU work must target WebGPU.
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
