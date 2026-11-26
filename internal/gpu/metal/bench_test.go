//go:build darwin && !ios

package metal

import (
	"image"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkRenderSprites draws frames of a 2D game at 1920×1080
// (gputest's SpriteScene: 1,872 sprites of one sheet) into one texture,
// waiting for each: ns/op is the time of a frame, GPU-ms what the GPU
// spent on it, by its command buffer's timestamps, and encode-ms what the
// CPU spent encoding it.
func BenchmarkRenderSprites(b *testing.B) {
	benchRender(b, gputest.SpriteScene(1920, 1080, 3, 3, 300))
}

// BenchmarkRenderSpritesZoomedOut draws the game zoomed out: about 7,000
// sprites.
func BenchmarkRenderSpritesZoomedOut(b *testing.B) {
	benchRender(b, gputest.SpriteScene(1920, 1080, 1.5, 3, 1000))
}

// BenchmarkRenderEmpty draws frames of nothing at 1920×1080: what the
// others spend besides their sprites.
func BenchmarkRenderEmpty(b *testing.B) {
	benchRender(b, &scene.Scene{Width: 1920, Height: 1080, Clear: scene.Color{A: 255}})
}

func benchRender(b *testing.B, s *scene.Scene) { benchRenderWith(b, s, nil) }

// benchRenderWith is benchRender with the renderer set up by setup, if not
// nil, and the batches of the frame reported.
func benchRenderWith(b *testing.B, s *scene.Scene, setup func(*Renderer)) {
	o, err := NewOffscreen(s.Width, s.Height)
	if err != nil {
		b.Skip("no Metal:", err)
	}
	defer o.Release()
	if setup != nil {
		setup(o.r)
	}
	var gpu, encoded time.Duration
	for b.Loop() {
		d, err := o.Render(s)
		if err != nil {
			b.Fatal(err)
		}
		gpu += d
		encoded += o.Encoded()
	}
	b.ReportMetric(gpu.Seconds()*1000/float64(b.N), "GPU-ms")
	b.ReportMetric(encoded.Seconds()*1000/float64(b.N), "encode-ms")
	if setup != nil {
		b.ReportMetric(float64(len(o.r.b.Batches)), "batches")
	}
}

// BenchmarkRenderBoxes draws 600 rounded boxes casting shadows at
// 1280×800, and BenchmarkRenderTranslatedBoxes the same boxes under
// translations, as an animation moves them.
func BenchmarkRenderBoxes(b *testing.B) { benchRender(b, gputest.BoxesScene(1280, 800, 600, false)) }

func BenchmarkRenderTranslatedBoxes(b *testing.B) {
	benchRender(b, gputest.BoxesScene(1280, 800, 600, true))
}

// BenchmarkRenderCards draws a dashboard of twelve cards with continuous
// corners at 2560×1600 (gputest.CardsScene), each casting two shadows,
// which the builder draws as nine-patches (gpu.Builder.SplitCorners).
func BenchmarkRenderCards(b *testing.B) { benchRender(b, gputest.CardsScene(2560, 1600)) }

// BenchmarkRenderPanels draws a page of 96 cards at 2560×1600
// (gputest.PanelsScene): clips and images change from card to card, so a
// frame takes hundreds of batches, and encode-ms is mostly their cost.
func BenchmarkRenderPanels(b *testing.B) {
	benchRenderWith(b, gputest.PanelsScene(2560, 1600, 96), func(*Renderer) {})
}

// BenchmarkVideoFrameRGBA draws a new 1920×1080 frame of video each frame,
// as video drew before frames had surfaces: converted to RGBA on the CPU,
// as a decoder in Go would, and uploaded, 8 MB a frame.
func BenchmarkVideoFrameRGBA(b *testing.B) {
	img := gputest.NV12Frame(1920, 1080, scene.BT709)
	rgb := scene.NewImageRGBA(1920, 1080, nil)
	benchVideo(b, videoBenchScene(rgb), func() {
		rgb.Pix = img.Surface.RGBA(rgb.Pix, 1920, 1080)
		rgb.Changed()
	})
}

// BenchmarkVideoFrameNV12 draws a new 1920×1080 frame of video each frame
// from its NV12 planes in memory, as ui.Bitmap.UpdateYUV gives them: 3 MB
// uploaded, and converted in the shader.
func BenchmarkVideoFrameNV12(b *testing.B) {
	img := gputest.NV12Frame(1920, 1080, scene.BT709)
	benchVideo(b, videoBenchScene(img), img.Changed)
}

func videoBenchScene(img *scene.Image) *scene.Scene {
	return &scene.Scene{Width: 1280, Height: 720, Clear: scene.Color{A: 255}, Ops: []scene.Op{{
		Kind: scene.OpImage, Rect: scene.Rect{W: 1280, H: 720}, Image: img, Src: scene.Rect{W: 1920, H: 1080},
	}}}
}

// benchVideo draws s after next changed its frame, each time, reporting
// what the GPU and the encoding spent as benchRender does.
func benchVideo(b *testing.B, s *scene.Scene, next func()) {
	o, err := NewOffscreen(s.Width, s.Height)
	if err != nil {
		b.Skip("no Metal:", err)
	}
	defer o.Release()
	b.SetBytes(1920 * 1080 * 4)
	var gpu, encoded time.Duration
	for b.Loop() {
		next()
		d, err := o.Render(s)
		if err != nil {
			b.Fatal(err)
		}
		gpu += d
		encoded += o.Encoded()
	}
	b.ReportMetric(gpu.Seconds()*1000/float64(b.N), "GPU-ms")
	b.ReportMetric(encoded.Seconds()*1000/float64(b.N), "encode-ms")
}

// BenchmarkPipelinedUploads compares pipelined 2-slot frame encoding when an
// atlas glyph rect, an RGBA image, or a rotated clip mask updates in place on
// every frame. Without blit staging, updates serialize CPU encoding and GPU
// execution via waitLast().
func BenchmarkPipelinedUploads(b *testing.B) {
	run := func(b *testing.B, setup func(*scene.Scene) func(frame int)) {
		r, err := newRenderer()
		if err != nil {
			b.Skip("no Metal:", err)
		}
		defer r.Release()
		s := gputest.BoxesScene(1280, 800, 600, false)
		s.MaskAtlas = scene.NewAtlas(1, 128, 128)
		step := setup(s)
		var targets [ringSize]id
		pool(func() {
			for i := range targets {
				targets[i] = r.newTexture(s.Width, s.Height, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0)
			}
			for i := range ringSize {
				cb, err := r.encode(s, targets[i])
				if err != nil {
					b.Fatal(err)
				}
				send(cb, "commit")
				r.track(cb)
			}
			r.waitLast()
		})
		defer pool(func() {
			r.waitLast()
			for i := range targets {
				release(&targets[i])
			}
		})

		frame := 0
		var encoded time.Duration
		for b.Loop() {
			if step != nil {
				step(frame)
			}
			t0 := time.Now()
			pool(func() {
				cb, err := r.encode(s, targets[r.slot])
				if err != nil {
					b.Fatal(err)
				}
				send(cb, "commit")
				r.track(cb)
			})
			encoded += time.Since(t0)
			frame++
		}
		pool(func() { r.waitLast() })
		if r.err != nil {
			b.Fatal(r.err)
		}
		b.ReportMetric(encoded.Seconds()*1000/float64(b.N), "frame-ms")
	}

	b.Run("control", func(b *testing.B) {
		run(b, func(*scene.Scene) func(int) { return nil })
	})

	b.Run("atlas", func(b *testing.B) {
		run(b, func(s *scene.Scene) func(int) {
			ax, ay, ok := s.MaskAtlas.Alloc(32, 32)
			if !ok {
				b.Fatal("atlas alloc failed")
			}
			start := int32(len(s.Glyphs))
			s.Glyphs = append(s.Glyphs, scene.Glyph{
				X: 16, Y: 16, W: 32, H: 32,
				U: uint16(ax), V: uint16(ay), UW: 32, VH: 32,
				Color: scene.Color{R: 255, G: 255, B: 255, A: 255},
			})
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: start, End: start + 1})
			patch := make([]byte, 32*32)
			for i := range patch {
				patch[i] = 200
			}
			return func(frame int) {
				patch[0] = byte(frame)
				s.MaskAtlas.Put(ax, ay, 32, 32, patch, 32)
			}
		})
	})

	b.Run("image", func(b *testing.B) {
		run(b, func(s *scene.Scene) func(int) {
			img := scene.NewImageRGBA(256, 256, make([]byte, 256*256*4))
			s.Ops = append(s.Ops, scene.Op{
				Kind:  scene.OpImage,
				Rect:  scene.Rect{X: 16, Y: 16, W: 256, H: 256},
				Image: img,
				Src:   scene.Rect{W: 256, H: 256},
			})
			return func(frame int) {
				img.Pix[0] = byte(frame)
				img.Changed()
			}
		})
	})

	b.Run("clipmask", func(b *testing.B) {
		run(b, func(s *scene.Scene) func(int) {
			idx := len(s.Ops)
			xform := scene.Translation(32, 32).Mul(scene.Rotation(6))
			s.Ops = append(s.Ops,
				scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{W: 900, H: 480}, Radii: scene.Uniform(16), Transform: xform},
				scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 900, H: 480}, Color: scene.Color{R: 40, G: 120, B: 240, A: 180}, Transform: xform},
				scene.Op{Kind: scene.OpPopClip},
			)
			return func(frame int) {
				s.Ops[idx].Rect.H = 480 + float32(frame%2)*2
			}
		})
	})
}

// BenchmarkRenderPartialCaret compares triple-buffered Metal rendering of a
// breathing caret over a dense multi-line text document: full-scene rebuilds &
// uploads vs Scene.Partial + incremental Builder + Part.Compact uploads.
func BenchmarkRenderPartialCaret(b *testing.B) {
	atlas := scene.NewAtlas(1, 512, 512)
	mx, my, _ := atlas.Alloc(8, 12)
	makeScene := func() (*scene.Scene, scene.PartEdit, int) {
		s := &scene.Scene{
			Width: 2560, Height: 1600, MaskAtlas: atlas, ID: 1,
			Stops: []scene.Stop{{Color: scene.Color{A: 255}, At: 0}, {Color: scene.Color{A: 0}, At: 1}},
		}
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: 2560, H: 1600}, Color: scene.Color{R: 250, G: 250, B: 250, A: 255}})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPushClip, Rect: scene.Rect{X: 200, Y: 80, W: 1600, H: 1440}})
		for row := range 80 {
			start := int32(len(s.Glyphs))
			for col := range 60 {
				s.Glyphs = append(s.Glyphs, scene.Glyph{
					X: float32(220 + col*11), Y: float32(100 + row*18),
					W: 8, H: 12, U: uint16(mx), V: uint16(my), UW: 8, VH: 12,
					Color: scene.Color{R: 30, G: 30, B: 30, A: 255},
				})
			}
			s.Ops = append(s.Ops, scene.Op{Kind: scene.OpGlyphs, Start: start, End: int32(len(s.Glyphs))})
		}
		opStart := len(s.Ops)
		caretIdx := len(s.Ops)
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 300, Y: 300, W: 3, H: 22}, Radii: scene.Uniform(1), Color: scene.Color{R: 20, G: 100, B: 240, A: 255}})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 200, Y: 80, W: 1600, H: 32}, Paint: scene.PaintLinearStops, Gradient: [4]float32{200, 80, 200, 112}, Stops: [2]int32{0, 2}})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 200, Y: 1488, W: 1600, H: 32}, Paint: scene.PaintLinearStops, Gradient: [4]float32{200, 1488, 200, 1520}, Stops: [2]int32{0, 2}})
		opEnd := len(s.Ops)
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpPopClip})
		s.Ops = append(s.Ops, scene.Op{Kind: scene.OpFill, Rect: scene.Rect{X: 1900, Y: 60, W: 120, H: 28}, Radii: scene.Uniform(6), Color: scene.Color{R: 80, G: 80, B: 90, A: 255}})
		edit := scene.PartEdit{
			OpStart: opStart, OldOpEnd: opEnd, NewOpEnd: opEnd,
			GlyphMark: len(s.Glyphs), StopMark: len(s.Stops),
		}
		return s, edit, caretIdx
	}

	for _, partial := range []bool{false, true} {
		name := "full"
		if partial {
			name = "partial"
		}
		b.Run(name, func(b *testing.B) {
			r, err := newRenderer()
			if err != nil {
				b.Skip("no Metal:", err)
			}
			defer r.Release()
			s, edit, caretIdx := makeScene()
			r.cur, r.wide = &r.formats[0], false
			var targets [3]id
			pool(func() {
				for i := range targets {
					targets[i] = r.newTexture(s.Width, s.Height, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0)
				}
				// Prime all 3 targets so Ages tracks them.
				for i := range targets {
					r.waitLast()
					r.cur = &r.formats[0]
					damage, known := r.diff.Diff(s)
					cb, err := r.drawFrame(s, damage, known, func() id { return targets[i] })
					if err != nil {
						b.Fatal(err)
					}
					send(cb, "commit")
					r.track(cb)
					send(cb, "waitUntilCompleted")
				}
			})
			defer pool(func() {
				r.waitLast()
				for i := range targets {
					release(&targets[i])
				}
			})

			var encoded time.Duration
			uploaded := 0
			frame := 0
			for b.Loop() {
				s.Ops[caretIdx].Color.A = byte(1 + frame%250)
				if partial {
					edit.BaseID = s.ID
					s.ID = uint64(frame + 2)
					s.Partial = edit
				} else {
					s.ID = uint64(frame + 2)
					s.Partial = scene.PartEdit{}
				}
				tex := targets[frame%len(targets)]
				t0 := time.Now()
				var encDur time.Duration
				pool(func() {
					r.waitLast()
					r.cur = &r.formats[0]
					var damage []image.Rectangle
					var known bool
					if partial {
						damage, known = r.diff.Diff(s)
					} else {
						r.forgetFrames()
						damage, known = r.diff.Diff(s)
					}
					cb, err := r.drawFrame(s, damage, known, func() id { return tex })
					if err != nil {
						b.Fatal(err)
					}
					send(cb, "commit")
					r.track(cb)
					encDur = time.Since(t0)
					send(cb, "waitUntilCompleted")
				})
				encoded += encDur
				if r.redrew == redrewPart {
					uploaded += len(r.part.Instances)
				} else {
					uploaded += len(r.drawState.b.Instances)
				}
				frame++
			}
			b.ReportMetric(encoded.Seconds()*1000/float64(b.N), "encode-ms")
			b.ReportMetric(float64(uploaded)/float64(b.N), "instances/op")
		})
	}
}

// BenchmarkRenderTargetCanvas measures creating, drawing, and blurring a
// window-sized transition canvas and its GPU-only scratch/backdrop targets.
func BenchmarkRenderTargetCanvas(b *testing.B) {
	const w, h = 2520, 1700
	r, err := newRenderer()
	if err != nil {
		b.Skip("no Metal:", err)
	}
	defer r.Release()

	target := id(0)
	pool(func() {
		target = r.newTexture(w, h, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0)
	})
	defer pool(func() {
		r.waitLast()
		release(&target)
	})

	layer := scene.NewCanvas(w, h)
	layer.Scene.Ops = []scene.Op{
		{Kind: scene.OpFill, Rect: scene.Rect{W: w, H: h}, Color: scene.Color{R: 245, G: 243, B: 238, A: 255}},
		{Kind: scene.OpFill, Rect: scene.Rect{X: 120, Y: 120, W: w - 240, H: h - 240}, Radii: scene.Uniform(20), Color: scene.Color{R: 32, G: 80, B: 200, A: 220}},
	}
	s := &scene.Scene{
		Width:    w,
		Height:   h,
		Clear:    scene.Color{A: 255},
		Canvases: []*scene.Image{layer},
		Ops: []scene.Op{
			{Kind: scene.OpImage, Rect: scene.Rect{W: w, H: h}, Image: layer, Src: scene.Rect{W: w, H: h}, Blur: 16},
		},
	}

	var mode, blurMode uint
	var gpuBytes uint64
	b.ReportAllocs()
	for frame := 0; b.Loop(); frame++ {
		if frame%2 == 0 {
			layer.Resize(w, h)
		} else {
			layer.Resize(w/2, h/2)
		}
		layer.Scene.Reset(layer.W, layer.H, scene.Color{})
		layer.Scene.Ops = append(layer.Scene.Ops,
			scene.Op{Kind: scene.OpFill, Rect: scene.Rect{W: float32(layer.W), H: float32(layer.H)}, Color: scene.Color{R: 245, G: 243, B: 238, A: 255}},
		)
		s.Ops[0].Src = scene.Rect{W: float32(layer.W), H: float32(layer.H)}
		pool(func() {
			r.waitLast()
			r.cur = &r.formats[0]
			cb, err := r.encode(s, target)
			if err != nil {
				b.Fatal(err)
			}
			send(cb, "commit")
			r.track(cb)
			send(cb, "waitUntilCompleted")
			if c := r.canvases[layer.ID()]; c != nil && c.tex != 0 {
				mode = uint(send(c.tex, "storageMode"))
			}
			if len(r.blurred) > 0 && r.blurred[0].tex != 0 {
				blurMode = uint(send(r.blurred[0].tex, "storageMode"))
			}
			gpuBytes = uint64(send(r.device, "currentAllocatedSize"))
		})
	}
	b.ReportMetric(float64(gpuBytes)/(1024*1024), "gpu-MB")
	b.ReportMetric(float64(mode), "canvas-mode")
	b.ReportMetric(float64(blurMode), "blur-mode")
}

// BenchmarkTransitionCanvasReap measures GPU canvas churn when transition
// layers are allocated and explicitly released across frames.
func BenchmarkTransitionCanvasReap(b *testing.B) {
	r, err := newRenderer()
	if err != nil {
		b.Skip("no Metal:", err)
	}
	defer r.Release()

	target := id(0)
	pool(func() {
		target = r.newTexture(512, 512, pixelFormatBGRA8Unorm, usageRenderTarget|usageShaderRead, nil, 0)
	})
	defer pool(func() {
		r.waitLast()
		release(&target)
	})

	var prev *scene.Image
	maxMap := 0
	b.ReportAllocs()
	for b.Loop() {
		if prev != nil {
			prev.Release()
		}
		c := scene.NewCanvas(512, 512)
		c.Scene.Ops = []scene.Op{
			{Kind: scene.OpFill, Rect: scene.Rect{W: 512, H: 512}, Color: scene.Color{R: 30, G: 90, B: 200, A: 255}},
		}
		s := &scene.Scene{
			Width: 512, Height: 512, Clear: scene.Color{A: 255},
			Canvases: []*scene.Image{c},
			Ops:      []scene.Op{{Kind: scene.OpImage, Rect: scene.Rect{W: 512, H: 512}, Image: c, Src: scene.Rect{W: 512, H: 512}}},
		}
		pool(func() {
			r.waitLast()
			r.cur = &r.formats[0]
			cb, err := r.encode(s, target)
			if err != nil {
				b.Fatal(err)
			}
			send(cb, "commit")
			r.track(cb)
			send(cb, "waitUntilCompleted")
			r.reapCanvases()
		})
		if n := len(r.canvases); n > maxMap {
			maxMap = n
		}
		prev = c
	}
	b.ReportMetric(float64(maxMap), "max-canvases")
}
