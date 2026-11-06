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

`MetalCanvas.Resize(w, h)` changes the target's pixel dimensions. The
renderer replaces the texture and calls `draw` again at the new size on
the next frame. The canvas keeps the same image identity, so existing
bitmaps and references remain valid.

For a responsive viewport, size the target from the element's DIP box and
`Context.PixelsPerDIP`:

```go
box := viewport.LayoutBounds()
scale := c.PixelsPerDIP()
canvas.Resize(
	int(math.Ceil(float64(box.W*scale))),
	int(math.Ceil(float64(box.H*scale))),
)
```

Resizing is idempotent when the pixel size is unchanged.

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
the result is composited back as an ordinary image. Native transforms on
the source element, including `RotateY` and `Perspective`, map pointer and
keyboard interaction along with the visual result. Coordinate changes
made only inside the Metal shader do not remap interaction.

## Imported textures

`NewMetalTexture` accepts an `MTLTexture` that the application already
owns:

```go
texture := ui.NewMetalTexture(width, height, frame.Device, externalTexture, owner)
ui.Image(c, texture.Bitmap()).Fill()
```

The renderer samples the texture directly; it does not upload pixels or
copy the texture. The native texture must be on the renderer's device,
use RGBA8 with premultiplied alpha, and remain alive while it is shown.
`owner` is retained with the image for a Go-side wrapper such as a cgo
handle. Call `SetTexture` when the producer recreates a texture of the
same size, and `Invalidate` when its contents change.

When the producer writes on another command buffer or queue, pass
`MetalTextureSync` so MyGo can order the read:

```go
texture := ui.NewMetalTextureWithSync(
	width, height, device, nativeTexture, owner,
	ui.MetalTextureSync{
		Wait:   encodeSharedEventWait,
		Signal: encodeSharedEventSignal,
	},
)
```

`Wait` runs once before MyGo first samples the texture in a frame's
command buffer. `Signal` runs after every sample of the texture in that
command buffer has been encoded. Both callbacks receive the renderer's
device, command buffer and native texture. They may encode an
`MTLSharedEvent` wait/signal or a fence, but must not commit or wait on
the command buffer. A Wait error fails the frame. `SetSync` replaces the
callbacks, and `SetTextureWithSync` replaces the texture and its
synchronization together.

The application still owns the native texture and its synchronization
objects. MyGo retains the Go `owner`, does not wait for the GPU to finish
sampling, and retires a replaced owner only after frames in flight that
could read the old texture have completed.

`BenchmarkRenderExternalTexture` samples a synchronized 512×512 imported
texture into 1920×1080 and reports CPU encode time and GPU timestamps:

```sh
go test -run '^$' -bench BenchmarkRenderExternalTexture -benchmem ./internal/gpu/metal
```

On the current Apple M5 Pro, the offscreen frame is about 0.37 ms wall
time, 0.013 ms encoding and 0.21 GPU-ms. The baseline is for regression
comparison, not a cross-device promise.

## Scope

This is a same-device, same-process Metal seam. The application must use
the device handed to it, and it must not commit the command buffer or
wait for GPU completion. MyGo owns the command buffer, presentation and
target lifetime. Imported textures remain owned by the application.

On a renderer that is not Metal, or when the window falls back to CPU
drawing, the bitmap is transparent. The API supports same-device Metal
textures, not cross-device or cross-API imports yet. It does not change
hit testing, focus, input methods or accessibility geometry for
shader-only coordinate changes. Those are separate composition and
interaction boundaries.

See [GPU backend direction](gpu-future.md) for the portability boundary
and the criteria for adding WebGPU or another backend.

The [Metal canvas example](../../examples/metal-canvas) uses cgo only in
the example to encode a compute shader. The framework's default build
remains `CGO_ENABLED=0`; the example has a pure-Go fallback for builds
without cgo.
