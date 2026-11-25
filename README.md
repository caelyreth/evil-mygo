### Evil MyGo

`evil-mygo` is a standalone downstream fork of [egoist/mygo](https://github.com/egoist/mygo) for building fast, low-overhead native Go desktop applications.

> [!NOTE]
> The original fork with custom WebGPU and GPU surface features has been moved to the [`classic`](https://github.com/caelyreth/evil-mygo/tree/classic) branch.

#### Highlights

- **Silent idle windows**: Breathing text carets and pulsing UI indicators (`Painter.OverlayFill`, `ui.Pulse`, `platform.OverlaySurface`) are offloaded to Core Animation on macOS and DirectComposition on Windows, eliminating background swapchain presents when nothing else is changing and falling back automatically when occluded, clipped cleanly to corners, or transformed.
- **Instant localized repaints**: Editing text, moving carets, or updating live status indicators diffs, batches, and uploads only the changed region (`307×` faster diffing, `119×` faster GPU batch building with `99.9%` fewer uploaded instances, and `8.9×` faster Metal encoding / `3.2×` faster total frames across Metal, D3D11, and OpenGL, with zero periodic full-window redraws).
- **Smooth large feeds & high-Hz scrolling**: Streaming items into virtualized 10,000-row logs, chats, or file trees runs `34.5×–3,264×` faster with `0–3` allocations, while sweeping a 120 Hz pointer over thousands of rows runs `248.6×` faster by culling clipped offscreen elements once per layout.
- **Responsive typing, IME & vector UI**: Positioning CJK and accented IME candidate popups drops from full-paragraph re-segmentation to cached binary search (`10.7×` faster, `99.95%` less heap churn); long wrapped paragraphs past 8,192 runes typeset linearly by window and incrementally around edits (`7.8×–15.5×` faster long-paragraph edits); glyph cache lookups and macOS rasterization are pooled; reusable vector icons/shapes run `5.76×` faster under transforms via `ui.BakePath`; and live theme/syntax recoloring skips text re-layout via `Painter.TextLayoutColors`.
- **Stutter-free GPU uploads, glass & leaner memory**: Metal atlas, image, and software NV12 video updates stream through ring-staged blit buffers without stalling in-flight GPU work (`2.19×` faster atlas-growth frames, `2.0–2.5×` faster video command encoding), single-pass mip-pyramid glass blur with IGN dithering runs `3.08×` faster without banding, and 1-byte clip masks cut mask RAM/VRAM `4×` (`2.91×` faster small cells) alongside a `128 MiB` per-renderer image cache cap.
- **Focused native-only footprint**: WebView runtimes, JS/TS bridges, Node/Bun scaffolding, and bundled app widgets are removed while keeping the native UI core, GPU backends, `plugins/glass`, updater, native modules, and cross-platform targets intact.

See [BENCHMARKS.md](/BENCHMARKS.md) for the full benchmark tables and methodology, and [DIFFERENCES.md](/DIFFERENCES.md) for the complete categorized list of additions, UI/platform features, sub-2× optimizations, and removals from upstream.

### License

Copyright (c) 2026 Yu (Caelyreth)
Copyright (c) 2026 MyGo contributors

`evil-mygo` is licensed under the [MIT License](/LICENSE).
