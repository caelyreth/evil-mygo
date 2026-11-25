package gpu

import (
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/scene"
)

// BenchmarkBuildSprites turns the scene of a 2D game's frame at 1920×1080
// into instances: tiles 3 times as large under 300 sprites, all of one
// sheet, which make one draw call.
func BenchmarkBuildSprites(b *testing.B) { benchBuild(b, 3, 3, 300) }

// BenchmarkBuildSpritesZoomedOut is the same game zoomed out to tiles 1.5
// times as large, under 1,000 sprites: about 7,000 instances.
func BenchmarkBuildSpritesZoomedOut(b *testing.B) { benchBuild(b, 1.5, 3, 1000) }

func benchBuild(b *testing.B, scale float32, layers, sprites int) {
	s := gputest.SpriteScene(1920, 1080, scale, layers, sprites)
	var bld Builder
	image := func(*scene.Image) uintptr { return 1 }
	for b.Loop() {
		bld.Build(s, image)
	}
	b.ReportMetric(float64(len(bld.Instances)), "instances")
	b.ReportMetric(float64(len(bld.Batches)), "batches")
}

// BenchmarkBuildTranslatedBoxes turns 600 rounded boxes casting shadows,
// each under a translation, into instances, the translations folded into
// them (BenchmarkBuildTranslatedBoxesUnfolded: under their transforms).
func BenchmarkBuildTranslatedBoxes(b *testing.B) { benchBuildBoxes(b, true, false) }

func BenchmarkBuildTranslatedBoxesUnfolded(b *testing.B) { benchBuildBoxes(b, true, true) }

// BenchmarkBuildBoxes turns the same boxes placed without transforms into
// instances.
func BenchmarkBuildBoxes(b *testing.B) { benchBuildBoxes(b, false, false) }

func benchBuildBoxes(b *testing.B, translated, noFold bool) {
	s := gputest.BoxesScene(1280, 800, 600, translated)
	bld := Builder{Classes: true, ClipSkip: true, NoFold: noFold}
	image := func(*scene.Image) uintptr { return 1 }
	for b.Loop() {
		bld.Build(s, image)
	}
	b.ReportMetric(float64(len(bld.Instances)), "instances")
	b.ReportMetric(float64(len(bld.Batches)), "batches")
}

// BenchmarkDiffRows compares a frame of a list of 200 rows at 3456×2000
// with the last (scene.Differ), as Metal's renderer does before drawing
// what changed: unchanged, and with one row's badge changing color, and
// BenchmarkBuildRows builds it, for comparison.
func BenchmarkDiffRows(b *testing.B) {
	for _, change := range []bool{false, true} {
		name := "unchanged"
		if change {
			name = "one_row"
		}
		b.Run(name, func(b *testing.B) {
			s := gputest.RowsScene(3456, 2000, 200)
			d := scene.Differ{Reach: 2}
			d.Diff(s)
			d.Remember(s)
			for i := 0; b.Loop(); i++ {
				if change {
					s.Ops[len(s.Ops)/2].Color.R = byte(i)
				}
				d.Diff(s)
				d.Remember(s)
			}
			b.ReportMetric(float64(len(s.Ops)), "ops")
			b.ReportMetric(float64(len(s.Glyphs)), "glyphs")
		})
	}
}

func BenchmarkBuildRows(b *testing.B) {
	s := gputest.RowsScene(3456, 2000, 200)
	bld := Builder{Classes: true, ClipSkip: true, Opaque: true}
	image := func(*scene.Image) uintptr { return 1 }
	for b.Loop() {
		bld.Build(s, image)
	}
	b.ReportMetric(float64(len(bld.Instances)), "instances")
}

// BenchmarkBuildPartialCaret measures Builder + Pipelines + Part.Compact for a
// single breathing caret update inside a large multi-line document scene.
func BenchmarkBuildPartialCaret(b *testing.B) {
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

	image := func(*scene.Image) uintptr { return 1 }
	for _, partial := range []bool{false, true} {
		name := "full"
		if partial {
			name = "incremental_compact"
		}
		b.Run(name, func(b *testing.B) {
			s, edit, caretIdx := makeScene()
			d := scene.Differ{Reach: 2}
			bld := Builder{Classes: true, ClipSkip: true, Opaque: true}
			var pipes Pipelines
			var part Part
			d.Diff(s)
			bld.Build(s, image)
			pipeFn := func(i int) uint64 { return uint64(bld.Batches[i].Classes()) }
			pipes.Damage(&bld, pipeFn, &d, nil, false)
			d.Remember(s)
			pipes.Remember()

			instTotal := 0
			partTotal := 0
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				s.Ops[caretIdx].Color.A = byte(1 + i%250)
				if partial {
					edit.BaseID = s.ID
					s.ID = uint64(i + 2)
					s.Partial = edit
				} else {
					s.ID = uint64(i + 2)
					s.Partial = scene.PartEdit{}
				}
				dmg, known := d.Diff(s)
				bld.Build(s, image)
				dmg = pipes.Damage(&bld, pipeFn, &d, dmg, known)
				instTotal += len(bld.Instances)
				if partial && part.Set(dmg, s.Width, s.Height, nil) {
					part.Compact(&bld, d.OpBound)
					partTotal += len(part.Instances)
				} else {
					partTotal += len(bld.Instances)
				}
				d.Remember(s)
				pipes.Remember()
			}
			b.ReportMetric(float64(instTotal)/float64(b.N), "built-instances")
			b.ReportMetric(float64(partTotal)/float64(b.N), "part-instances")
		})
	}
}
