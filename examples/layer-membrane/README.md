# Layer Membrane

An independent native `ui.UILayer` sampled by a WebGPU scene and presented
on a moving, thin physical membrane. The field, text input, button, focus
state, IME caret and cursor all remain native MyGo UI; the app owns only
the 3D scene and its surface mapping.

```sh
CGO_ENABLED=1 go run ./examples/layer-membrane
```

The shader renders a finite membrane with a front face, a solid fabric back,
and visible edge walls. The CPU hit-testing path and the WGSL shader use the
same camera basis, aspect ratio, wave function, root refinement, ground
occlusion and front-face test. A pointer ray therefore lands on the same UV
that the GPU samples. Input is routed through `UILayer.HandleSurfaceInput`;
`Element.HostLayer` bridges focus, IME caret geometry, cursor state and the
accessibility subtree through the moving surface.

## Controls

- `WASD` walks; `Q`/`E` changes height. Camera movement pauses while the
  inner text field owns text input.
- Drag the viewport away from a native control for mouse look.
- The wheel changes the camera distance.
- `SEND SIGNAL` is inside the rendered layer, not an overlay.
- `Surface depth` changes both the visible waves and the CPU hit surface.

The example intentionally keeps cloth physics, a scene graph and camera code
in the application. `ui` supplies the native layer, WebGPU device/encoder,
capture source, pointer routing, focus, IME and accessibility.

For the cgo-free WebGPU path:

```sh
CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/layer-membrane
```
