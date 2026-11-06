# Flagscape

A WebGPU 3D panel inside a native MyGo window. The left viewport is a
Metal-backed WebGPU surface; the right side is native MyGo UI.

```sh
go run ./examples/flagscape
```

The panel face is a live MyGo subtree captured by `Element.Effect` and
sampled as `WebGPUFrame.Source`. It is not a CPU screenshot: the source
stays in a WebGPU texture, and the panel shader renders it with lighting,
a grid, and a thick front/back surface.

## Controls

- `WASD` moves the camera; `Q`/`E` changes height.
- Drag the viewport for mouse look; the wheel changes camera distance.
- The large `ACTIVATE` control is hit-tested through the panel's camera
  ray and UVs. The outer sidebar shows the activation count.

The scene and input mapping are application code. `ui` supplies the
captured native layer, the WebGPU device/encoder, pointer routing, focus,
IME and accessibility.
