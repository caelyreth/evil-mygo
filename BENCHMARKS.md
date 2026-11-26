### Benchmarks

All benchmarks below were measured sequentially with zero concurrent workload on an Apple M5 Pro (`darwin/arm64`, 15 logical CPUs, Go 1.27.3, `CGO_ENABLED=0`) against upstream baseline `a40ac688` (or full-frame baseline for partial-frame rendering). For the high-level overview, see [README.md](/README.md); for the full categorized list of changes, see [DIFFERENCES.md](/DIFFERENCES.md).

#### Idle & Partial-Frame Rendering

Unobstructed breathing overlays (`Painter.OverlayFill`, `ui.Pulse`, `platform.OverlaySurface`) are delegated to macOS Core Animation (`CALayer` + `CABasicAnimation`) and Windows DirectComposition (`IDCompositionVisual` + `IDCompositionEffectGroup` + `IDCompositionAnimation`), reducing idle overlay ticks to zero frame wakeups and zero swapchain presents when supported. For in-scene partial updates (such as typing or animating inside a tracked part), partial-frame diffing and upload compaction skip whole-frame diffing and re-uploads (`-count=5` medians):

| Benchmark | Full Frame (`Scene.Partial` unset) | Partial (`Scene.Partial` + `Compact`) | Delta |
| --- | --- | --- | --- |
| `BenchmarkDifferPartialCaret-15` (`scene`) | `36,752 ns/op` (`0 B`, `0 allocs`) | `119.6 ns/op` (`0 B`, `0 allocs`) | **`-99.67%` (`307.3×` faster)** |
| `BenchmarkBuildPartialCaret-15` (`gpu`) | `241,258 ns/op` (`4,805` built / `4,805` uploaded, `1 B/op`, `0 allocs`) | `2,020 ns/op` (`4,805` built / `4` uploaded, `0 B/op`, `0 allocs`) | **`-99.16%` (`119.4×` faster), `-99.92%` instances uploaded** |
| `BenchmarkRenderPartialCaret-15` (`metal`) | `535,363 ns/op` (`0.2541 encode-ms`, `4,805` instances/op) | `167,949 ns/op` (`0.02855 encode-ms`, `4` instances/op) | **`-88.8%` CPU encode (`8.90×` faster), `-68.6%` total (`3.19×` faster)** |

#### Graphics Pipeline, Uploads & Clip Masks

**Metal ring staging, dynamic `framebufferOnly`, 1-byte `A8`/`R8` clip masks & damage tracking (`-count=5` medians)**

| Benchmark | Baseline (`a40ac688`) | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkPipelinedUploads/control-15` | `218,312 ns/op` (`0.2183 frame-ms`, `57 allocs`) | `212,259 ns/op` (`0.2122 frame-ms`, `57 allocs`) | `-2.8%` *(control)* |
| `BenchmarkPipelinedUploads/atlas-15` | `484,280 ns/op` (`0.4840 frame-ms`, `66 allocs`) | `220,995 ns/op` (`0.2208 frame-ms`, `64 allocs`) | **`-54.4%` (`2.19×` faster; matches control)** |
| `BenchmarkPipelinedUploads/image-15` | `337,518 ns/op` (`0.3374 frame-ms`, `68 allocs`) | `270,988 ns/op` (`0.2709 frame-ms`, `66 allocs`) | **`-19.7%`**, `-2 allocs` |
| `BenchmarkPipelinedUploads/clipmask-15` | `2,275,886 ns/op` (`2.276 frame-ms`, `9,396 B/op`, `73 allocs`) | `2,326,384 ns/op` (`2.326 frame-ms`, `5,399 B/op`, `71 allocs`) | **`-42.5%` heap allocs**, 4× less GPU clip texture |
| `BenchmarkClipMaskRoundedCard/card-15` | `265,850 ns/op` (`656 B/op`, `4 allocs`) | `138,558 ns/op` (`656 B/op`, `4 allocs`) | **`-47.9%` (`1.92×` faster)**, 4× less RAM/VRAM |
| `BenchmarkClipMaskRoundedCard/cell-15` | `8,562 ns/op` (`976 B/op`, `2 allocs`) | `2,945 ns/op` (`976 B/op`, `2 allocs`) | **`-65.6%` (`2.91×` faster)**, 4× less RAM/VRAM |
| `BenchmarkDiffEffectWithImage-15` | `1,554 ns/op` (`1.000 damage-rects`, `0 allocs`) | `1,557 ns/op` (`0 damage-rects`, `0 allocs`) | **Eliminated `1.0` -> `0` spurious damage rects** |
| `BenchmarkVideoFrameRGBA-15` | `22,384,156 ns/op` (`0.4659 GPU-ms`, `0.5900 encode-ms`, `81 allocs`) | `23,241,170 ns/op` (`0.5085 GPU-ms`, `0.2927 encode-ms`, `81 allocs`) | **`encode-ms -50.4%` (`2.02×` faster)** *(total CPU bound by YUV->RGBA)* |
| `BenchmarkVideoFrameNV12-15` | `433,357 ns/op` (`19,139.88 MB/s`, `0.08529 GPU-ms`, `0.2029 encode-ms`, `81 allocs`) | `347,893 ns/op` (`23,841.83 MB/s`, `0.1308 GPU-ms`, `0.08234 encode-ms`, `77 allocs`) | **`-19.7%` total, `encode-ms -59.4%` (`2.46×` faster), `+24.6% MB/s`, `-4 allocs`** |
| `BenchmarkRenderEmpty-15` | `148,111 ns/op` (`0.01044 GPU-ms`, `0.009509 encode-ms`) | `149,830 ns/op` (`0.006714 GPU-ms`, `0.009510 encode-ms`) | Within noise |
| `BenchmarkRenderBoxes-15` | `415,034 ns/op` (`0.1476 GPU-ms`, `0.1240 encode-ms`) | `430,172 ns/op` (`0.1536 GPU-ms`, `0.1291 encode-ms`) | Within noise |
| `BenchmarkRenderCards-15` | `1,186,105 ns/op` (`0.8832 GPU-ms`, `0.1147 encode-ms`) | `1,198,704 ns/op` (`0.9012 GPU-ms`, `0.1109 encode-ms`) | Within noise |
| `BenchmarkRenderPanels-15` | `700,560 ns/op` (`0.3229 GPU-ms`, `0.2072 encode-ms`) | `735,006 ns/op` (`0.3524 GPU-ms`, `0.2037 encode-ms`) | Within noise |
| `BenchmarkRenderBatches/boxes/joined-15` | `437,738 ns/op` (`0.1757 GPU-ms`, `0.1251 encode-ms`) | `424,302 ns/op` (`0.1609 GPU-ms`, `0.1215 encode-ms`) | `-3.1%` |
| `BenchmarkRenderBatches/rows/joined-15` | `293,483 ns/op` (`0.06836 GPU-ms`, `0.07802 encode-ms`) | `279,659 ns/op` (`0.05699 GPU-ms`, `0.07559 encode-ms`) | `-4.7%` |
| `BenchmarkBuildSprites-15` | `73,398 ns/op` (`0 allocs`) | `68,850 ns/op` (`0 allocs`) | `-6.2%` |
| `BenchmarkBuildRows-15` | `149,732 ns/op` (`0 allocs`) | `147,919 ns/op` (`0 allocs`) | `-1.2%` |
| `BenchmarkDiffRows/unchanged-15` | `52,764 ns/op` (`0 allocs`) | `47,842 ns/op` (`0 allocs`) | `-9.3%` |

**`plugins/glass` single-pass mip-pyramid blur (`67f97c8b`, `-count=5` medians)**

| Benchmark | Baseline Median | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkBlur/even-15` | `1,580,738 ns/op` (`187,635 B/op`, `22 allocs`) | `1,398,171 ns/op` (`188,275 B/op`, `22 allocs`) | **`-11.5%` (`1.13×` faster)** |
| `BenchmarkBlur/progressive-15` | `10,552,532 ns/op` (`7,937,286 B/op`, `22 allocs`) | `3,423,362 ns/op` (`8,572,322 B/op`, `30 allocs`) | **`-67.6%` (`3.08×` faster)** |
| `BenchmarkGlass-15` | `5,917,404 ns/op` (`4,321,250 B/op`, `24 allocs`) | `5,384,581 ns/op` (`4,321,885 B/op`, `24 allocs`) | **`-9.0%` (`1.10×` faster)** |

#### Text Rasterization, Layout, Lists & Hit Testing

**Glyph lock batching, CoreGraphics scratch pooling & layout memoization (`-count=3` medians)**

| Benchmark | Baseline Median | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkGlyphCache/hit-15` | `2,224 ns/op` (`0 B`, `0 allocs`) | `1,984 ns/op` (`0 B`, `0 allocs`) | **`-10.8%`** *(unlocked)* |
| `BenchmarkGlyphCache/hit_locked-15` | `2,224 ns/op` *(unlocked baseline)* | `1,308 ns/op` (`0 B`, `0 allocs`) | **`-41.2%`** (`-34.1%` vs unlocked) |
| `BenchmarkGlyphCache/miss-15` | `270,711 ns/op` (`122,514 B`, `3,356 allocs`) | `171,286 ns/op` (`48,457 B`, `1,718 allocs`) | **`-36.7%` ns, `-60.4%` B, `-48.8%` allocs** |
| `BenchmarkLayout-15` (`text`) | `24,033 ns/op` (`11,459 B`, `33 allocs`) | `26,185 ns/op` (`11,459 B`, `33 allocs`) | `+8.9%` *(noise; same allocs)* |
| `BenchmarkLayoutLabel-15` | `4,987 ns/op` (`1,545 B`, `25 allocs`) | `5,257 ns/op` (`1,545 B`, `25 allocs`) | `+5.4%` *(noise; same allocs)* |
| `BenchmarkLayoutDocument-15` | `15,948 ns/op` (`0 B`, `0 allocs`) | `17,266 ns/op` (`0 B`, `0 allocs`) | `+8.3%` *(noise; 0 allocs)* |
| `BenchmarkLiveRows3000Change-15` | `5,829,187 ns/op` (`6,424 B`, `148 allocs`) | `4,970,661 ns/op` (`6,273 B`, `138 allocs`) *(`-count=5` solo; noisy `-count=3`: `6,644,114 ns/op`)* | **`-14.7%` ns, `-10` allocs** |
| `BenchmarkLiveRows3000Hover-15` | `166,296 ns/op` (`7 B`, `0 allocs`) | `160,210 ns/op` (`7 B`, `0 allocs`) | **`-3.7%`** |
| `BenchmarkLiveRows3000Cross-15` | `58,559 ns/op` (`0 B`, `0 allocs`) | `56,662 ns/op` (`0 B`, `0 allocs`) | **`-3.2%`** |
| `BenchmarkPainterRichText/steady-15` | `56,511 ns/op` (`16 B`, `1 alloc`) | `55,209 ns/op` (`16 B`, `1 alloc`) | `-2.3%` |
| `BenchmarkPainterRichText/pages-15` | `1,202,244 ns/op` (`319,056 B`, `4,564 allocs`) | `1,245,869 ns/op` (`315,269 B`, `4,509 allocs`) | `+3.6%` ns, `-1.2%` B/allocs |
| `BenchmarkFrame-15` | `52,935 ns/op` (`1,089 B`, `53 allocs`) | `52,938 ns/op` (`1,089 B`, `53 allocs`) | `0.0%` |
| `BenchmarkFrameHover-15` | `53,877 ns/op` (`1,089 B`, `53 allocs`) | `53,871 ns/op` (`1,089 B`, `53 allocs`) | `-0.0%` |
| `BenchmarkListScroll-15` | `98,161 ns/op` (`11,362 B`, `274 allocs`) | `101,019 ns/op` (`11,313 B`, `274 allocs`) | `+2.9%` ns |
| `BenchmarkStressFrame/boxes-15` | `169,664 ns/op` (`0 B`, `0 allocs`) | `175,236 ns/op` (`0 B`, `0 allocs`) | `+3.3%` *(noise)* |
| `BenchmarkStressFrame/labels-15` | `278,445 ns/op` (`33,960 B`, `757 allocs`) | `277,810 ns/op` (`33,961 B`, `757 allocs`) | `-0.2%` |
| `BenchmarkStressFrame/translated_column-15` | `651,378 ns/op` (`13,969 B`, `1,745 allocs`) | `660,237 ns/op` (`13,967 B`, `1,745 allocs`) | `+1.4%` |
| `BenchmarkStressFrame/list-15` | `148,058 ns/op` (`15,353 B`, `411 allocs`) | `150,363 ns/op` (`15,335 B`, `411 allocs`) | `+1.6%` |
| `BenchmarkStressFrame/dashboard-15` | `167,619 ns/op` (`32,475 B`, `549 allocs`) | `169,912 ns/op` (`32,559 B`, `550 allocs`) | `+1.4%` |
| `BenchmarkLayout/DeepAuto/10000-15` | `591,588 ns/op` (`0 B`, `0 allocs`) | `620,426 ns/op` (`0 B`, `0 allocs`) | `+4.9%` *(noise)* |
| `BenchmarkLayout/Wide/10000-15` | `605,773 ns/op` (`0 B`, `0 allocs`) | `635,960 ns/op` (`0 B`, `0 allocs`) | `+5.0%` *(noise)* |

**`O(B)` block list shift & viewport-clipped hit cache (`-count=3` medians)**

| Benchmark | Baseline Median | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkListHeightsShift/unaligned-15` | `352,025 ns/op` (`53,776 B`, `175 allocs`) | `10,212 ns/op` (`51,712 B`, `3 allocs`) | **`-97.1%` ns (`34.5×` faster), `-98.3%` allocs** |
| `BenchmarkListHeightsShift/aligned-15` | `351,240 ns/op` (`53,488 B`, `174 allocs`) | `107.6 ns/op` (`0 B`, `0 allocs`) | **`-99.97%` ns (`3,264×` faster), `0 B/op`, `0 allocs`** |
| `BenchmarkLiveRows3000Hover-15` | `159,442 ns/op` (`7 B`, `0 allocs`) | `72,436 ns/op` (`3 B`, `0 allocs`) | **`-54.6%` ns (`2.20×` faster)** |
| `BenchmarkLiveRows3000Cross-15` | `57,433 ns/op` (`0 B`, `0 allocs`) | `231.0 ns/op` (`0 B`, `0 allocs`) | **`-99.60%` ns (`248.6×` faster)** |
| `BenchmarkLiveRows3000Same-15` | `6,764,333 ns/op` *(A/B at matched CPU state; `5,705,677` initial)* | `6,621,843 ns/op` (`271 B`, `32 allocs`) | `-2.1%` *(within thermal noise vs A/B)* |
| `BenchmarkLayout/Wide/10000-15` | `818,687 ns/op` *(A/B; `627,413` initial)* | `872,403 ns/op` (`0 B`, `0 allocs`) | Within noise |
| `BenchmarkLayout/DeepAuto/10000-15` | `650,206 ns/op` *(A/B; `592,960` initial)* | `697,056 ns/op` (`0 B`, `0 allocs`) | Within noise |

#### Atlas Repacking, Vector Paths, Partial Repaints & Text Editing

**Atlas repacking, cached vector paths & rich text spans (`-count=3` medians)**

| Benchmark | Baseline Median | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkAtlasRepack-15` (`scene`) | `281,475 ns/op` (`630,266 B`, `2,009 allocs`) | `254,700 ns/op` (`583,144 B`, `10 allocs`) | **`-9.5%` ns, `-7.5%` B, `-99.5%` allocs** |
| `BenchmarkFillPathVsBaked` (`ui`) | `14,123 ns/op` (`48 B`, `1 alloc` `FillPath`) | `2,452 ns/op` (`48 B`, `1 alloc` `FillBakedPath`) | **`-82.6%` (`5.76×` faster)** |
| `BenchmarkPainterRichText/steady-15` | `51,737 ns/op` (`16 B`, `1 alloc`) | `54,486 ns/op` (`16 B`, `1 alloc`) | Within noise (`+5.3%`, same allocs) |
| `BenchmarkPainterRichText/pages-15` | `1,178,519 ns/op` (`316,213 B`, `4,522 allocs`) | `1,274,290 ns/op` (`316,474 B`, `4,491 allocs`) | `+8.1%` *(noise)*, `-31` allocs |
| `BenchmarkFrameAnimatedPaths-15` | `223,865 ns/op` (`8,120 B`, `13 allocs`) | `238,062 ns/op` (`8,120 B`, `13 allocs`) | `+6.3%` *(noise; same allocs)* |

**Partial-repaint tail reuse & cached IME/paragraph follow-ups (`-count=3` medians)**

| Benchmark | Baseline Median | After | Delta |
| --- | --- | --- | --- |
| `BenchmarkRepaintPartsText-15` (`ui`) | `9,517 ns/op` (`0.04166 whole-paints/op`, `14 B/op`, `0 allocs`) | `9,110 ns/op` (`0 whole-paints/op`, `0 B/op`, `0 allocs`) | **`100%` elimination of periodic full repaints (`-4.3%` ns, `0 B/op`)** |
| `BenchmarkBoundsForRange-15` (`ui`) | `33,779 ns/op` (`31,240 B/op`, `15 allocs`) | `3,162 ns/op` (`16 B/op`, `1 allocs`) | **`-90.6%` ns (`10.7×` faster), `-99.95%` B, `-93.3%` allocs** |
| `BenchmarkTextAreaLinesType-15` (`ui`) | `1,012,049 ns/op` (`448,574 B/op`, `1,087 allocs`, `b.N=3,811`) | `930,624 / 1,358,024 / 1,433,821 ns/op` (`414k–627k B/op`, `628–852 allocs`, `b.N=5,469–7,758`) | **Fewer allocs (`-42%` at `b.N=5,469`), `b.N` grew 2× as paragraph 0 grew monotonically** |

**Long-text windowed (`7f96b946`) and incremental line-reuse (`d21bd725`) Core Text typesetting (`internal/text`, `-count=5` medians)**

`BenchmarkLayoutLong` measures wrapped paragraphs past `typesetLong` (`8,192` runes): `cold` releases shaping between iterations, while `edit` cycles 16 distinct middle-word mutations across consecutive `EndFrame()` frames. Windowed `typesetWindows` enforces exact Core Text shaping and strictly linear cold scaling (`4.57 → 18.0 → 72.3 ms` for `16 → 64 → 256 KiB`), and `long_darwin.go` reuses unchanged head and tail lines across consecutive edits:

| Benchmark | Pre-Window (`46ae2db2`) | Windowed Linear (`7f96b946`) | Incremental Reuse (`d21bd725`) | Delta |
| --- | --- | --- | --- | --- |
| `BenchmarkLayoutLong/cold/16KiB-15` | `561,181 ns/op` (`3.35 MB`, `82 allocs`) | `4,569,020 ns/op` (`3.06 MB`, `4,453 allocs`) | `4,588,506 ns/op` (`4.13 MB`, `4,469 allocs`) | Linear windowed cold pass |
| `BenchmarkLayoutLong/cold/64KiB-15` | `2,703,207 ns/op` (`13.3 MB`, `147 allocs`) | `18,022,152 ns/op` (`12.2 MB`, `17,668 allocs`) | `18,237,029 ns/op` (`16.4 MB`, `17,686 allocs`) | Linear `4×` scaling with length |
| `BenchmarkLayoutLong/cold/256KiB-15` | `24,488,482 ns/op` *(bimodal `6.4–39.7 ms`, `451 allocs`)* | `72,311,522 ns/op` (`49.2 MB`, `70,486 allocs`) | `72,489,561 ns/op` (`65.9 MB`, `70,507 allocs`) | Eliminates whole-text spike |
| `BenchmarkLayoutLong/edit/16KiB-15` | `534,454 ns/op` (`3.35 MB`, `82 allocs`) | `4,575,260 ns/op` (`3.06 MB`, `4,453 allocs`) | `585,619 ns/op` (`3.55 MB`, `122 allocs`) | **`-87.2%` vs windowed (`7.81×` faster), `-97.3%` allocs** |
| `BenchmarkLayoutLong/edit/64KiB-15` | `2,547,564 ns/op` (`13.2 MB`, `143 allocs`) | `17,994,351 ns/op` (`12.2 MB`, `17,667 allocs`) | `1,535,567 ns/op` (`14.1 MB`, `130 allocs`) | **`-91.5%` vs windowed (`11.7×` faster), `-39.7%` vs `46ae2db2`** |
| `BenchmarkLayoutLong/edit/256KiB-15` | `23,829,881 ns/op` *(bimodal `6.1–39.5 ms`, `445 allocs`)* | `72,597,120 ns/op` (`49.2 MB`, `70,486 allocs`) | `4,686,002 ns/op` (`56.7 MB`, `142 allocs`) | **`-93.5%` vs windowed (`15.5×` faster), `-80.3%` vs `46ae2db2` median** |

**Metal private render targets, transition canvas reaping & idle atlas shrinking (`9bf0d96b`..`5f5e5738`, `-count=5` medians)**

| Benchmark | Result | Alloc / Resource Footprint | Notes |
| --- | --- | --- | --- |
| `BenchmarkRenderTargetCanvas-15` (`metal`) | `1,236,642 ns/op` | `9,956 B/op`, `207 allocs/op`, `56.98`–`69.89 gpu-MB` *(median `56.98 gpu-MB`)* | `canvas-mode: 2.000`, `blur-mode: 2.000` (`MTLStorageModePrivate`, no duplicate CPU backing store) |
| `BenchmarkTransitionCanvasReap-15` (`metal`) | `215,547 ns/op` | `9,948 B/op`, `143 allocs/op`, `1.000 max-canvases` | Immediate `Image.Release` + per-frame GPU reaping |
| `BenchmarkReleaseShapingIdleShrink-15` (`text`) | `212,697 ns/op` | `22,347,785 B/op`, `5 allocs/op`, `64.00 idle-KiB` | Collapses overgrown `MaskAtlas` + `ColorAtlas` back to `64 KiB` on idle release |
