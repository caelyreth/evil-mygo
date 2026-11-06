# Metal path retired

The direct Metal renderer and the `ui.MetalCanvas` / `ui.MetalTexture`
composition API have been removed. The native GPU path is
[`go-webgpu`](webgpu.md), which uses wgpu-native and maps to Metal on
macOS.

`CGO_ENABLED=0` builds use the CPU native renderer unless they opt into the
cgo-free WebGPU path with `-tags mygo_webgpu,nofakecgo`.
