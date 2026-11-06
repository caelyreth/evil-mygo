# WebGPU layer

A native MyGo window whose left viewport is a WebGPU canvas sampling an
independent off-window `UILayer`.

```sh
go run ./examples/webgpu-layer
```

The texture is a persistent rendered native layer, not a screenshot. The
right sidebar is ordinary host-window UI; the left controls are rendered
inside the layer and remain interactive through `PlanarSurface` and
`HostLayer`, including focus, IME, cursor and accessibility routing.
