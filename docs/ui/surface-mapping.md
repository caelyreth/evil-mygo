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

A `SurfaceHit` marked as a back face is not routed into the UI by
`HandleSurfaceInput`; a surface that exposes a differently mapped back
face should return its own front-facing UV instead. The map must report
the first visible surface under the pointer so that occluded geometry
cannot take input.

`Version` identifies the geometry snapshot used for the pixels the user
saw. Do not use a newer or older deformation to resolve a click; that
produces the familiar "the button is beside the cursor" bug. This version
belongs to the surface, not to `UILayer.Version`, which identifies the
layer's rendered UI content. A host updates both atomically for the frame
it presents: the surface deformation and the UI snapshot may change
independently, but input must use the pair that was actually rendered.

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
`Size` and input coordinates stay in DIPs. Use `NewUILayerWithScale` when
the destination needs more backing pixels than one per DIP; `Scale`
reports the chosen density and the bitmap grows with it while interactions
remain in logical coordinates.
Use `ScreenToSurface` to turn a scene hit into a layer coordinate before
calling `HandleInput`. Call `NeedsFrame` after rendering if the layer's
view requested another frame. `Access`, `AccessByRole`, `TextCaret` and
`TextInputActive` expose the layer's accessibility and text-input state
to the host that presents it. `Version` identifies the current rendered UI
snapshot. It is independent of the version of the `SurfaceMap` used for
input.

`HandleSurfaceInput` combines the two steps and preserves pointer capture
while a press moves beyond the surface. It never performs GPU picking or
readback; the supplied `SurfaceMap` must be updated from the same scene
state that is being rendered. While a press is held, a mapped move updates
the last UV, an unmapped move keeps that UV, and a move back onto the
surface resumes live tracking. Canceled presses map to the engine's
pointer-cancel path and click nothing. Send `ui.InputPointerLeave` when the
pointer leaves the presenting viewport to clear layer hover; it does not
end an active capture. The outer host can read `UILayer.Cursor` after
input to show the cursor requested by the native element under the
mapped pointer.

Text input crosses the same boundary. Create one client per presented
surface and give it to the hosting element:

```go
text := layer.TextInputClient(surface)
view := ui.Box(c).Focusable().HandleTextInput(text)
view.HandleInput(func(ev ui.InputEvent) bool {
	handled := layer.HandleSurfaceInput(surface, ev)
	if handled {
		layer.Render()
	}
	return handled
})
```

The client forwards composition and editing into the layer. It projects
the inner caret's bounds back through the surface map and maps native
point queries back to the inner text layout. Keep the client stable while
the surface is presented; update the `SurfaceMap` before rendering the
frame it describes.

A `UILayer.Bitmap()` can also be passed to `MetalCanvas.SetSource`, so
the same live layer can be sampled by an external Metal pass without
first being attached to the window tree:

```go
layer := ui.NewUILayer(buildPanel, 900, 500)
metal := ui.NewMetalCanvas(900, 500, drawMetalPass)
metal.SetSource(layer.Bitmap())
```

`UILayer`, `MetalCanvas`, and `MetalTexture` all satisfy the
backend-neutral `ui.GPUTexture` contract (`Size`, `Bitmap`,
`Invalidate`). It gives host code a common handle for composition and
lifecycle without hiding the backend-specific constructor needed to
create the resource.

The layer benchmarks are:

```sh
go test -run '^$' -bench 'BenchmarkUILayer' -benchmem ./ui
```

On the current macOS arm64 development machine, the small layer benchmark
is about 3.5 µs for a render and 0.36 µs for a down/up surface-input pair,
with six allocations each. These are measurement baselines, not promises
for a particular scene or GPU.
