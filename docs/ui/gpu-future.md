# GPU backend direction

Metal is the reference interoperability backend. The framework now has
five pieces that are independent of a future graphics API:

- `ui.GPUTexture` is the common bitmap-like composition handle.
- `ui.MetalCanvas` owns a render target and exposes a frame command buffer
  to application code.
- `ui.MetalTexture` imports an application-owned same-device texture.
- `MetalTextureSync` orders imported reads with a producer.
- `ui.SurfaceMap`, `ui.UILayer` and `Handle.HandleLayerInput` connect GPU
  presentation to native hit testing, IME and accessibility.

The generic contract deliberately does not expose a raw device pointer.
Raw handles stay in backend-specific types because their type, ownership,
pixel format and synchronization rules differ.

## Why WebGPU is the preferred successor

WebGPU offers one explicit resource and synchronization model over Metal,
Vulkan and Direct3D 12. Its texture-view and queue-submission model maps
more directly to the contracts above than OpenGL does, and it leaves room
for a browser path without inventing another application API.

It is not being added speculatively. Integration still needs a maintained
native implementation and Go binding, predictable binary cost, texture
format negotiation, shared-device import and a synchronization story
equivalent to `MetalTextureSync`. That work should start only when a
second native backend is an actual product requirement.

## Why not Vulkan first

Vulkan has strong native engine interop on Linux and Windows, but macOS
requires MoltenVK and its translation layer. A Vulkan-first generic API
would either expose Metal and MoltenVK differences or force every host to
live with the extra layer. Vulkan remains a good optional native
interop backend for apps that already own a Vulkan device; it is not the
smallest cross-platform core.

## Port checklist

A second GPU backend should provide:

1. A pixel render-target constructor and a frame callback whose command
   buffer is owned by MyGo.
2. Same-device imported textures with explicit lifetime retirement and
   producer/consumer synchronization.
3. A reusable off-window UI texture path with no CPU readback.
4. The same `SurfaceMap` input, cursor, IME and accessibility contracts.
5. Opt-in registration so apps that do not use the backend carry no
   implementation or dependency cost.
6. CPU/GPU benchmarks for encode time, GPU time, input mapping and surface
   compositing on each supported platform.

The `ui` layer must not learn any scene graph, cloth solver or renderer
specific type while adding that backend.
