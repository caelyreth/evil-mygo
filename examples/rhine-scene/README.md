# RHINE LAB / Material Archive

A native MyGo material archive: procedural three-dimensional files, live native
UI on both sides of an extracted file, and an industrial instrument HUD over a
zero-copy WebGPU scene. No webview, JavaScript runtime, external model data or
additional Go dependencies are required.

```sh
nix-shell -p go --run 'CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/rhine-scene'
```

Requires macOS and the same local wgpu-native runtime as the other WebGPU
examples. A normal cgo-enabled build also works.

## Startup And Navigation

The window opens to a native startup screen. **Open archive** creates the mesh,
scene controller, GPU canvas and dossier layers; waiting on this screen does not
render the archive or advance its animation. The opening wave begins with the
first scene GPU frame, runs once for 5.4 seconds, and stops at an interactive
file preview. It excludes time hidden and never automatically opens a file.

- Click a file to select it. Its column settles into a wave and the file rises
  to preview height; the outgoing file returns to its slot.
- Double-click, press Enter with the scene focused, or choose **Examine** to
  extract the file and move the camera closer.
- **Return**, Escape, or a click outside the examined file returns to preview.
  An outside click never selects another file at the same time.
- **Flip** orbits an examined file between its front and back. It appears after
  the report reveal completes and the camera close-in is nearly settled. Its
  screen position stays anchored through flips and changes only on reopening
  or resizing.
- Arrow keys and the compact bottom-left column/row controls navigate the
  catalogue. Vertical and horizontal scrolling navigate rows and columns.
- **Near / Far** selects close framing or an elevated, wider survey. Changing
  this preset returns an examined file to preview.
- **Light / Dark** changes the whole scene and native UI. The amber underlight
  stays warm, with a restrained reduction in saturation and output in Dark.

There are 33,280 selectable files in 65 columns of 512 rows. Every input path
clamps to the same bounds. Non-selectable guard files conceal the catalogue
edges. A rolling 21-by-193 field follows the rendered camera; view/fog culling
retains visible files and nearby shadow casters. Simulation state outside that
field is retired once its extracted files have returned.

The entrance can be interrupted from its current rendered pose. Camera,
extraction, lighting and native text/control changes animate smoothly. Hidden
windows pause motion, and frame requests stop once everything settles.

## Procedural Geometry And Lighting

Every file is a bonded three-layer laminate: two frosted-polymer slabs and one
opaque content slab. All three are 0.14 units thick, have the same footprint,
and touch without air gaps. Rounded rectangular rings form their bevels,
closed sidewalls and end faces. The same pure-Go model builder creates the
index inlay and mirrored titanium screw heads, including rounded cross recesses
and closed recess floors. Mesh generation happens once on **Open archive**;
the resulting geometry and material buffers are reused by every file instance.

Physical index inlays use stable file-ID-seeded procedural pigments: muted
gold, forest green, rust, or slate blue. Their fine grain belongs to the mesh,
not an overlay, and does not reseed when files or the camera move.

The scene uses physical surface shading, soft directional shadows, analytic
studio reflections, frosted refraction, distance haze, depth-aware lens blur,
and ACES color output. Both polymer slabs contribute to an optical-depth
accumulation pass, so draw order cannot erase an overlapping cover. Refraction
taps reject nearer opaque geometry. The optical pass reuses the opaque pass's
multisample scratch buffer.

An amber strip follows the selected slot with damped position changes, just
below the preview's lower edge. It is depth-tested world-space geometry in the
ground pass. The strip, illuminated insert and floor enter the refraction
capture; the polymer also integrates warm light through its physical thickness.
The source stays under the slot when a file is extracted and has finite reach.

The studio key has a fixed world-space direction. Shadow coverage follows the
rendered camera on a stable texel grid and fades at its boundary. Focus distance
also eases between selections. Dark mode blends the studio key/fill, reflections,
floor, haze and native UI together; it does not apply a screen-wide tint.

The lighting environment is analytic, not a prefiltered environment map.
Transmission uses optical-depth-weighted composition rather than recursive ray
tracing. Full screen-space ambient occlusion and a separate bloom pass are not
implemented.

## Native File Surfaces

Each face has its own `ui.UILayer`, UI engine, focus, controls and session
records. A native image-only atlas layer packs their GPU bitmaps side by side
for `WebGPUCanvas.SetSource`. The scene samples the appropriate atlas half on
each actual core face before the frosted covers refract it.

Both faces use MyGo's public `ui.ProjectiveSurface`, `Element.HostLayer` and
`Handle.HandleLayerInput` APIs. The reverse map sits on the negative-Z face
with an outward-facing 180-degree rotation, so its text reads normally and its
input does not reuse the front's UV map. The scene's existing raycast handles
occlusion. Maps and rendering share one prepared camera/instance snapshot.
Only the visible, settled face accepts input; flipping cancels pointer capture
and suspends face input until the orbit settles. Native focus, keyboard, IME,
text editing, cursors and accessibility remain owned by MyGo.

The front presents a dedicated cover while browsing. A stable avalanche hash
of the file ID selects one of four visual families: laminate qualification,
surface inspection, transmission assay, or process certification. Their cover
compositions, technical diagrams and report summaries differ. Examination
waits for three quarters of the extraction before transforming the cover into
an interactive report:

- **Summary:** specimen identity, material profile and optical measurements.
- **Analysis:** wavelength slider, comparison trace and simulated sample scans.
- **Notes:** editable laboratory notes with save/discard actions.
- **Review:** mark a scanned file reviewed; another scan clears that decision.

The reverse presents handling and provenance: storage humidity, seal
verification, handling hold, a custody-transfer log and separate handling notes.
These controls never modify the front's analysis, review or notes. All records
are demonstration data retained per file for the current session, not a
persistent document store or live telemetry.

The native pages reserve a 68-DIP top hardware rail, 52-DIP side gutters and a
48-DIP bottom gutter in their 1000-by-740 logical surfaces. The layout clears
the index inlay and both screw heads.

Navigation fades the outgoing cover on its original file before displaying the
next one. A quintic endpoint curve blends each fully rendered page once against
its physical surface in linear light. During examination, the specimen register
moves into the report header and the report unfolds into tabs, content and
actions. Return reverses that motion. Interrupted transitions preserve their
current position and velocity rather than restarting an effect.

## HUD

The scene fills the entire window, including behind the macOS traffic lights.
An invisible top-edge drag region allows window movement without a separate
header. The industrial palette combines navy (`#141c2c`), cream (`#f4e0bc`),
gold (`#dca854`), forest green (`#3f5b4b`) and rust (`#8c3c38`). Borderless blocks,
bold tracked typography, slash screens, halftone and ordered dither are native
drawing commands rather than bitmap decorations.

The upper-right selected-file register uses compact typography and print
textures. Five equal-width digit cells keep the first digit centered in the
gold band. Column and row controls share the bottom-left corner; camera and
lighting controls share the bottom-right. Changing numbers roll, action labels
crossfade, and button colors and poses ease between states. Native MyGo motion
honors reduced-motion preferences.

## GPU Ownership

MyGo provides the WebGPU device, command encoder and destination texture.
The example records every pass into that encoder; MyGo samples the result in
its native UI pass and submits the frame. No CPU pixel readback, screenshot
upload, cross-device texture transfer, extra graphics context or separate queue
submission is used. Only resize reallocates GPU targets. GPU captures in
`renderer_test.go` are test-only facilities.

## Source Layout

- `model.go`, `model_geometry.go`: geometry types and deterministic mesh builder.
- `motion.go`, `layout.go`: entrance curves, transforms, cameras and lighting globals.
- `controller.go`, `archive.go`: shared state, springs, bounded residency and picking.
- `startup.go`: idle welcome screen and explicit scene creation.
- `hud.go`, `hud_motion.go`, `hud_texture.go`, `hud_turn.go`: native overlay and motion.
- `appearance.go`, `brand.go`, `assets/rhine-lab.svg`: palettes and vector emblem.
- `dossier.go`, `dossier_cover.go`, `dossier_variants.go`: front covers and reports.
- `dossier_back.go`: independent custody and handling surface.
- `dossier_motion.go`, `dossier_surface.go`: transitions and projective input mapping.
- `scene.wgsl`, `finish.wgsl`, `renderer.go`: persistent multi-pass HDR rendering.
- `webgpu_pass.go`: local purego depth-attachment ABI adapter for go-webgpu v0.5.5.

## Notices

Third-party license notices are retained in `THIRD_PARTY_LICENSES`. Names and
marks belong to their respective owners; the local vector emblem is not an
official branding asset.
