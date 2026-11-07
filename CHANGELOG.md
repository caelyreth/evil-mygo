# Changelog

## 0.1.1

Sync [egoist/mygo from 442d09c293e9 through 94f22c384d0e](https://github.com/egoist/mygo/compare/442d09c293e9586809175fe55beff0be905459c7...94f22c384d0ecd8486899e5a4662cbec2577d310),
advancing the recorded upstream version from 0.8.0 to 0.8.3.

- Add richer drawn menus, SVG menu icons, popup placement and accessibility.
- Add Markdown alerts, mentions and issue references; list-row hover/press
  queries; `Tester.AccessAll`; and `Page.GPUProcessID`.
- Add opt-in AAC-LC, FLAC and MP4/Apple Lossless audio support. Importing
  `audio/aac` links an LGPL-2.1-or-later decoder; other apps do not link it.
- Bring in text shaping, Korean wrapping, CJK font fallback, IME, editor,
  updater, Linux click-through and fetch URL fixes.
- Adopt clip-mask memory reuse and nested rounded clipping, and reclaim idle
  WebGPU image textures. Adapt perspective transforms to WebGPU's scene and
  effect shaders, including backdrop sampling.
- Preserve GPU-only image resizing and native offscreen-layer accessibility.

The direct Metal backend stays retired. Upstream release numbers do not replace
the fork's versions. The upstream Swift native-module feature remains reverted.
The two upstream showcase entries and their images are included.

## 0.1.0

Establish the `github.com/caelyreth/evil-mygo` module, scoped npm packages and
independent fork versioning, retaining the WebGPU renderer, interactive native
UI surfaces, silk flag and procedural material archive.
