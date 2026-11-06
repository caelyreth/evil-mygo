# Flagscape

A static 3D panel inside a native Metal viewport. The left side is the
scene; the right side is a compact grayscale MyGo sidebar.

```sh
CGO_ENABLED=1 go run ./examples/flagscape
```

The panel face is live MyGo UI captured by `MetalCanvas.Effect`. It is
rendered as a thin slab with front, back and side surfaces, so moving
behind or around it still shows a solid panel.

## Controls

- `WASD` moves the camera; `Q`/`E` lowers and raises it.
- Click and drag the viewport to capture the pointer for mouse look.
- The wheel moves the camera along Z.
- The large `ACTIVATE` control is hit-tested through the 3D surface: the
  pointer ray produces panel UVs, which are checked against the native
  control's source-space rectangle.
- The outer sidebar shows the activation count.

The panel does not animate and has no wind, cloth, or pole. `CGO_ENABLED=0`
builds the native sidebar but leaves the Metal viewport blank.
