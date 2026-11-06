# Metal Layer

This example demonstrates the second half of the Metal experiment: a live
native MyGo subtree becomes a GPU texture that an application Metal
compute pass transforms, and the result is composited back as a native
image.

```sh
CGO_ENABLED=1 go run ./examples/metal-layer
```

The source card is built with ordinary MyGo widgets. `Element.Cache` keeps
its native layer stable when only the Metal pass animates;
`MetalCanvas.Effect` captures it, sets the captured texture as
`MetalFrame.Source`, and paints the Metal target in its place. The compute
shader in `layer_draw_darwin.m` reads the source and writes the target.
No CPU readback or WebView is involved.

The source controls still use their original geometry for pointer input,
focus, IME and accessibility. The effect bends pixels only; it does not
remap interaction. That limitation is intentional in this experiment.

Without cgo the example builds and runs, but the Metal target is blank.
The framework itself remains a `CGO_ENABLED=0` build; cgo is isolated to
the macOS example encoder.
