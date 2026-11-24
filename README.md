### Evil MyGo

`evil-mygo` is a standalone downstream fork of [egoist/mygo](https://github.com/egoist/mygo) for native Go UI and desktop applications.

> [!NOTE]
> The original fork with custom WebGPU and GPU surface features has been moved to the [`classic`](https://github.com/caelyreth/evil-mygo/tree/classic) branch.

- **Performance & memory**: On-demand `TextLayout` shaping release (`Release`/`Shaped`), `CacheScaled` kept-layer raster scale retention, exact clip-transform cancellation, lazy `ui.Tester` frames, and `Context.AssistiveTechnology()`.
- **UI & menus**: Scale watch and exit-ghost fixes, suppressed pre-sized initial resize frames, native/drawn palette swatch menu rows, and `ui.FlyoutMenus` popovers with backdrop blur, rise animation, and separator coloring.
- **Platform & tooling**: `MYGO_LOCALE` AppKit launch language, `macos.icon` asset catalogs in `mygo dev`, and Windows Known Folders / XDG user directory resolution.
- **Native-only scope**: Stripped WebView runtimes, JS/TS bridges, Node/Bun/TypeScript scaffolding, bundled widgets (`ui/markdown`, `ui/editor`, `ui/richtext`, `ui/chart`, `ui/video`, `ui/netimage`), `audio/`, docs, examples, and unused plugins while keeping the native UI runtime, `plugins/glass`, updater, native modules, and platform targets intact.

See [DIFFERENCES.md](/DIFFERENCES.md) for the full categorized list of changes and ongoing divergences from upstream.

### License

Copyright (c) 2026 Yu (Caelyreth)
Copyright (c) 2026 MyGo contributors

`evil-mygo` is licensed under the [MIT License](/LICENSE).
