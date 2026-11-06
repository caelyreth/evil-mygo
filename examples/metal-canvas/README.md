# Metal Canvas

A native MyGo window with a custom Metal compute target and normal MyGo
controls around it. The compute pass writes into a texture owned by MyGo's
Metal renderer; MyGo samples that texture in the same command buffer. No
CPU readback or upload is involved.

```sh
# On macOS, with cgo and the system Metal frameworks:
CGO_ENABLED=1 go run ./examples/metal-canvas

# Pure-Go build: the native UI still runs, but the Metal target is blank.
CGO_ENABLED=0 go run ./examples/metal-canvas
```

The Metal encoder is in `metal_draw_darwin.m`. The Go side uses
`ui.NewMetalCanvas`; `MetalFrame` carries the native device, command
buffer and render target. The surface is an ordinary `ui.Bitmap`, so it
can be placed, clipped, transformed and surrounded by the rest of the
native tree.

The callback runs when the canvas is first shown and after
`MetalCanvas.Invalidate`. The demo invalidates on every view rebuild; the
animation additionally requests frames. It is intentionally a small
same-device, same-process experiment, not a cross-device texture import
API.
