# Flagscape

A full-bleed Metal 3D viewport on the left and native MyGo controls on the
right. The scene is raymarched in a Metal compute shader; the flag's cloth
lighting and wave are driven by the camera and wind uniforms.

```sh
CGO_ENABLED=1 go run ./examples/flagscape
```

The flag texture is a live MyGo face captured by `MetalCanvas.Effect`, not
an image asset. The right panel is ordinary MyGo UI. `Element.Cache`
keeps the face's source texture stable when only the cloth or camera
changes.

## Controls

- `WASD` walks the camera; `Q`/`E` lower and raise it.
- Click and drag the viewport to capture the pointer for mouse look.
- The wheel zooms the camera.
- `Cloth motion`, wind, sunlight and `Reset view` are native controls.

The pointer capture is MyGo's native press capture: the viewport takes the
press, so moves and release continue to it outside the surface. It is not
a global system pointer lock.

`CGO_ENABLED=0` builds and runs the native panel but leaves the Metal
viewport blank; cgo is isolated to this example encoder.
