# GPU backend direction

`github.com/go-webgpu/webgpu` is the native GPU boundary. It uses
wgpu-native, maps to Metal on macOS, and owns the portable composition
contracts:

- `ui.GPUTexture`, `ui.WebGPUCanvas`, and `ui.WebGPUTexture` provide
  reusable GPU composition and same-device texture import.
- `ui.SurfaceMap`, `ui.UILayer`, `UILayer.HandleSurfaceInput` and
  `Handle.HostLayer` connect GPU presentation to hit testing, pointer
  capture, IME, cursor and accessibility.
- The WebGPU scene renderer covers native UI canvases, image atlases,
  shader effects, backdrops, separable blur and the glass plugin.

The generic contract deliberately does not expose a raw device pointer.
Backend-backed applications receive `*wgpu` handles through
`ui.WebGPUFrame`; the rest of the UI tree stays independent of the
renderer.

## Why WebGPU

WebGPU offers one explicit resource and synchronization model over
wgpu-native's Metal, Vulkan and Direct3D backends. Its texture-view and
queue-submission model maps directly to MyGo's composition contracts and
leaves room for a browser path without inventing another application API.

## Why not Vulkan first

Vulkan has strong native engine interop on Linux and Windows, but macOS
requires MoltenVK and its translation layer. A Vulkan-first generic API
would either expose Metal and MoltenVK differences or force every host to
live with the extra layer. Vulkan remains a good optional native
interop backend for apps that already own a Vulkan device; it is not the
smallest cross-platform core.

## Remaining checklist

The remaining work is:

1. Remove the `nofakecgo` tag from cgo-free builds after goffi's ARM64
   callback path accepts the struct arguments and returns used by AppKit.

The `ui` layer must not learn any scene graph, cloth solver or renderer
specific type while adding that backend.
