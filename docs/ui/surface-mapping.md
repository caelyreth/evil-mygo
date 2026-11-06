# Surface mapping

When native UI is presented through a custom Metal scene, the visible
pixels and the input geometry are separate. `ui.SurfaceMap` is the
framework contract that connects them:

```go
type SurfaceMap interface {
	Version() uint64
	ScreenToSurface(Point) (SurfaceHit, bool)
	SurfaceToScreen(UV) (Point, bool)
}
```

`ScreenToSurface` returns the UV under a window point, its depth and
whether the hit is a back face. `SurfaceToScreen` projects UI positions,
carets and focus geometry back onto the surface. Implementations for
cloth, heightfields, meshes, or fluid simulations belong to the package
that owns that surface.

The `Version` must identify the geometry snapshot used for the pixels
the user saw. Do not use a newer or older deformation to resolve a click;
that produces the familiar “the button is beside the cursor” bug.

`PlanarSurface` handles an axis-aligned native panel. `ProjectiveSurface`
handles a rectangular panel through `ui.Transform`, including
`RotateY`/`Perspective`; its inverse rejects back faces and singular
projections.

The core does not contain a cloth or fluid solver. A surface package can
provide a coarse CPU collision proxy and return UVs from the same
simulation state used by its renderer.

## Live UI layers

`ui.NewUILayer` runs independent native UI off-window and exposes its
output as a bitmap:

```go
layer := ui.NewUILayer(func(c *ui.Context) {
	ui.TextInput(c, &name)
	ui.Button(c, "Apply")
}, 900, 500)

layer.HandleInput(ui.InputEvent{
	Kind: ui.InputPointerDown,
	X: local.X,
	Y: local.Y,
	Button: 0,
})
layer.Render()
```

The layer has its own layout, focus, IME, scrolling, pointer capture,
and accessibility state. `Render` snapshots the display list, so later
frames do not mutate a texture that another compositor is still showing.
Use `ScreenToSurface` to turn a scene hit into a layer coordinate before
calling `HandleInput`. Call `NeedsFrame` after rendering if the layer's
view requested another frame. `Access`, `AccessByRole`, `TextCaret` and
`TextInputActive` expose the layer's accessibility and text-input state
to the host that presents it.

`HandleSurfaceInput` combines the two steps and preserves pointer capture
while a press moves beyond the surface. It never performs GPU picking or
readback; the supplied `SurfaceMap` must be updated from the same scene
state that is being rendered.
