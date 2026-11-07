# Layer Membrane / Silk Study

An independent native `ui.UILayer` sampled by a WebGPU scene and presented
on a single ivory silk flag in a dark, gridded wind tunnel. The field, text
input, button, focus state, IME caret and cursor all remain native MyGo UI;
the app owns only the 3D scene and its surface mapping.

```sh
nix-shell -p go --run 'CGO_ENABLED=1 go run ./examples/layer-membrane'
```

The old ray-traced solid panel is replaced entirely. The flag is an open,
two-sided 120-by-80 quad mesh, with no offset back shell or extruded edge
walls. Its left edge is fixed; traveling folds, small ripples and a little
sag grow toward the free edge. Integrating unit tangents along each row
keeps its horizontal strip length constant instead of stretching a flat
heightfield. The flag's proportions are independent of the viewport.

WebGPU rasterizes the triangles with a depth buffer and 4x MSAA, so folds,
grazing views and floor occlusion do not depend on a ray solver converging.
Both faces stay light, with directional satin highlights, filtered weave,
a narrow hem and muted ink showing through the reverse. The floor uses a
derivative-filtered grid that fades into the dark background.

The pinned go-webgpu v0.5.5 has native layout mismatches. This example uses
shader-inferred bindings and storage-buffer vertex pulling to avoid those
paths; `webgpu_pass.go` supplies the missing v29 depth-attachment chain field
through purego. These workarounds stay local to the example.

CPU hit testing intersects those exact same uploaded triangles, using tiled
bounds to skip empty space and choosing the nearest visible hit. Projected
carets also check for occlusion by other folds. Input is routed through
`UILayer.HandleSurfaceInput`; `Element.HostLayer` bridges focus, IME caret
geometry, cursor state and the accessibility subtree through the moving
surface.

## Controls

- `WASD` walks; `Q`/`E` changes height. Camera movement pauses while the
  inner text field owns text input.
- Drag the viewport away from a native control for mouse look.
- The wheel changes the camera distance.
- `SEND SIGNAL` is inside the rendered layer, not an overlay.
- `Wind strength` changes the folds of the shared render/input mesh.
- `Wind motion` pauses or resumes the flag without changing the camera.

The example keeps procedural cloth motion, mesh geometry and camera code
in the application; it is not a physical cloth simulation. `ui` supplies
the native layer, WebGPU device/encoder, capture source, pointer routing,
focus, IME and accessibility.

For the cgo-free WebGPU path:

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/layer-membrane'
```
