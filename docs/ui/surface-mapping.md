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
