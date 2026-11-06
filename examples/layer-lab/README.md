# Optics Lab

A native-only playground for `ui.Effect` and `ui/shader`'s `layerSample`.
The specimen is a real MyGo component: editable track title, seek slider,
play button and save button, alongside vector artwork and native text.

```sh
go run ./examples/layer-lab
# Or, with Nix:
nix-shell -p go --run 'go run ./examples/layer-lab'
```

Choose crystal refraction, fluted/frosted glass, liquid ripples, chromatic
prisms or mosaic focus. Move over the specimen to position the lens;
turn off **Follow pointer** to pin it. Adjust strength, lens size and
dispersion. **Show original** bypasses the shader, and zero strength is
also an identity operation. **Animate light** pauses/resumes the clock
without a time jump. The player is a UI specimen, not an audio player.

## What this demonstrates

- `stage.Cache().Effect(...)` captures the native subtree into a reusable
  GPU layer. `optics.kage` only reads that layer with `layerSample`; it
  does not read the window backdrop, decode an image or use the glass plugin.
- `effects.go` sets pre-looked-up uniforms at paint time and requests
  paint-only animation at up to 60 FPS. The source cache avoids GPU
  rerasterization when the component's pixels have not changed; normal
  painting still records/compares its scene to discover changes.
- The glass uses edge refraction, magnification, separate RGB samples,
  directional highlights and a small frost kernel. Other modes change
  sampling coordinates or combine offset samples of the same tree.
- Effect coordinates are DIPs; capture resolution follows the display
  scale. Sampling is clamped to the source, so no extra capture margin
  is needed.

## Deliberate limitations

Shader effects do not change hit testing, focus, IME or accessibility
geometry. Controls still work at their original positions, not at a
displaced image of them. This makes the boundary of the existing effect
API visible rather than pretending to solve interactive GPU embedding.

The full-size shader is bypassed on software renderers and with Reduce
Transparency; the live native component still works. Reduce Motion
freezes animation. Pausing or bypassing the effect requests no continuous
frames (a focused text field may still blink its caret).

```sh
go test ./examples/layer-lab
# Optional automation for inspecting the real GPU window:
go build -tags e2e -o /tmp/mygo-layer-lab ./examples/layer-lab
MYGO_AUTOMATION=/path/to/private-dir/lab.sock /tmp/mygo-layer-lab
go run ./plugins/automation/cmd/mygo-automation open --socket /path/to/private-dir/lab.sock
```
