# Layer Membrane

An independent native `ui.UILayer` sampled by a Metal ray tracer and
presented on a moving heightfield. The field, text input, button, focus
state, IME caret and cursor all remain native MyGo UI; the app only owns
the 3D scene and its surface mapping.

```sh
CGO_ENABLED=1 go run ./examples/layer-membrane
```

The CPU and Metal sides use the same camera basis, aspect ratio, wave
function, root refinement, ground occlusion and front-face test. A
pointer ray therefore lands on the same UV that the GPU samples. Input is
routed through `UILayer.HandleSurfaceInput`; `Element.HostLayer` bridges
focus, IME caret geometry, cursor state and the accessibility subtree
through the moving surface.

## Controls

- `WASD` walks; `Q`/`E` changes height. Camera movement pauses while the
  inner text field owns text input.
- Drag the viewport away from a native control for mouse look.
- The wheel changes the camera distance.
- `SEND SIGNAL` is inside the rendered layer, not an overlay.
- `Surface depth` changes both the visible waves and the CPU hit surface.

The example intentionally does not put cloth physics, a scene graph, or
camera code in `ui`. Those are application concerns. It has a pure-Go
fallback so the native controls still build with `CGO_ENABLED=0`; the
Metal viewport is blank in that build.

## Verification

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go test ./examples/layer-membrane'
nix-shell -p go clang --run 'CGO_ENABLED=1 go test ./examples/layer-membrane'
```

The cgo test compiles the Metal Shading Language at runtime, encodes a
real offscreen frame, and checks that the pass produced image content.
