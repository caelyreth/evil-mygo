# WebGPU canvas

A native MyGo window whose left viewport is an application-owned WebGPU
render target. The callback receives the renderer's device, command
encoder and target texture, encodes a pass, and MyGo samples the result
in the same submitted frame. The controls around it are ordinary MyGo
input.

```sh
# A cgo-enabled macOS build uses WebGPU by default.
go run ./examples/webgpu-canvas

# A cgo-free build needs the shared fake-cgo runtime tag.
CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/webgpu-canvas
```

The renderer and application share the wgpu-native device and collection
queue. There is no readback or pixel upload; the target is an RGBA8
texture sampled by the native scene pass.
