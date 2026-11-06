# Metal canvases

`ui.NewMetalCanvas` creates a pixel render target that an application's
Metal code encodes into while MyGo draws a frame. The result is an
ordinary `ui.Bitmap`: it can be placed, clipped, transformed and surrounded
by native controls, but its pixels stay in a GPU texture. There is no CPU
readback or upload on the Metal path.

```go
canvas := ui.NewMetalCanvas(800, 450, func(f ui.MetalFrame) error {
	// f.Device, f.CommandBuffer and f.Texture are native Metal objects.
	// Encode into f.CommandBuffer. Do not commit or wait on it.
	renderMyScene(f)
	return nil
})

ui.Box(c).Fill().Children(func() {
	ui.Image(c, canvas.Bitmap()).Fill()
})
```

`MetalFrame` provides the renderer's `MTLDevice`, the frame's
`MTLCommandBuffer`, the target `MTLTexture`, its pixel size and the
display scale. The target is available to custom compute or render
passes. The callback runs on the main thread while MyGo encodes the
frame, before MyGo samples the target in the same command buffer.

The callback runs on first display and when `MetalCanvas.Invalidate` is
called. A view that rebuilds because its controls changed should
invalidate the canvas when those controls feed the Metal pass. An
animated canvas should call `Invalidate` and request a frame from the
view. A paused canvas can be invalidated once to render a new still
image without requesting continuous frames.

## Native UI as a source

`MetalCanvas.Effect` connects the other direction. It captures the
element and its children, sets the captured bitmap as `MetalFrame.Source`,
and paints the canvas in place of the element:

```go
canvas := ui.NewMetalCanvas(900, 500, drawMyMetalPass)

ui.Box(c).Fill().Cache().Effect(canvas.Effect()).Children(func() {
	ui.TextInput(c, &app.name)
	ui.Slider(c, &app.volume, 0, 1)
})
```

The source is a rendered native subtree, not a DOM tree. Its hit testing,
focus, text input and accessibility still use the original element
geometry. A custom renderer can sample `Source` and write the target;
the result is composited back as an ordinary image.

## Scope

This is a same-device, same-process Metal seam. The application must use
the device handed to it, and it must not commit the command buffer or
wait for GPU completion. MyGo owns the command buffer, presentation and
target lifetime.

On a renderer that is not Metal, or when the window falls back to CPU
drawing, the bitmap is transparent. This API does not import a texture
owned by another device or graphics API yet, and it does not change hit
testing, focus, input methods or accessibility geometry. Those are
separate composition and interaction boundaries.

The [Metal canvas example](../../examples/metal-canvas) uses cgo only in
the example to encode a compute shader. The framework's default build
remains `CGO_ENABLED=0`; the example has a pure-Go fallback for builds
without cgo.
