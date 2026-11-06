# Layer Surface

An independent `ui.UILayer` rendered through a custom Metal texture. The
left side is the GPU output; the right side is the host window's native
sidebar.

```sh
CGO_ENABLED=1 go run ./examples/layer-surface
```

The layer owns its own native layout, text field, button, focus, input
state, and accessibility tree. `MetalCanvas.SetSource(layer.Bitmap())`
feeds that layer to the Metal pass without attaching it to the window
tree. Pointer events on the left are mapped through a `PlanarSurface`
and routed with `UILayer.HandleSurfaceInput`.

Without cgo the native sidebar works, but the Metal target is blank.
