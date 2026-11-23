### Evil MyGo

`evil-mygo` is a standalone downstream fork of [egoist/mygo](https://github.com/egoist/mygo) for native Go UI and desktop applications.

> [!NOTE]
> The original fork with custom WebGPU and GPU surface features has been moved to the [`classic`](https://github.com/caelyreth/evil-mygo/tree/classic) branch.

- **UI & platform fixes**: Transition scale/exit-ghost fixes, on-demand `TextLayout` shaping release, assistive-technology awareness, kept-layer raster scaling, exact clip transforms, `MYGO_LOCALE` AppKit launch language, `macos.icon` dev support, Windows/XDG user folders, and deferred initial resize frames.
- **Menus & flyouts**: Palette swatch rows in system and drawn menus, plus `FlyoutMenu` popovers with material blur, rise animation, and separator coloring.
- **Native-only boundary**: Stripped the embedded WebView/browser runtime (`Page`, `WebView`, `WebView2`/WKWebView/WebKitGTK), JS/TS bridge & custom scheme handlers, and Node/Bun/TypeScript scaffolding and workspace clutter, keeping the native UI runtime, updater, and platform targets (`darwin`, `linux`, `windows`, `mobile`, `web` WASM) intact.
- **Trimmed scope**: Removed bundled app widgets and extras (`ui/markdown`, `ui/editor`, `ui/richtext`, `ui/chart`, `ui/video`, `ui/netimage`, `audio`, `document*`, unused plugins beyond `glass`, examples, docs, and website).

### License

Copyright (c) 2026 Yu (Caelyreth)
Copyright (c) 2026 MyGo contributors

`evil-mygo` is licensed under the [MIT License](/LICENSE).
