package ui

import "testing"

func testComplexGlyphPath(ox, oy float32) *Path {
	var p Path
	p.MoveTo(ox+2, oy+14)
	p.CubeTo(ox+2, oy+5, ox+8, oy+2, ox+14, oy+2)
	p.CubeTo(ox+20, oy+2, ox+24, oy+8, ox+22, oy+14)
	p.QuadTo(ox+18, oy+20, ox+12, oy+22)
	p.CubeTo(ox+6, oy+22, ox+2, oy+19, ox+2, oy+14)
	p.Close()
	// Inner counter (EvenOdd / opposite winding hole).
	p.MoveTo(ox+9, oy+12)
	p.LineTo(ox+15, oy+12)
	p.LineTo(ox+12, oy+8)
	p.Close()
	return &p
}

func TestFillBakedPathMatchesFillPath(t *testing.T) {
	baked := BakePath(testComplexGlyphPath(0, 0))
	white := RGB(230, 240, 250)
	bg := RGB(0, 0, 0)
	offsets := [][2]float32{
		{0, 0},
		{10.25, 8.5},
		{15.75, 11.25},
		{10.3, 14.3},
	}

	for _, scale := range []float32{1, 2} {
		for _, off := range offsets {
			ox, oy := off[0], off[1]
			cmpX, cmpY := ox, oy
			if ox == 10.3 {
				// Non-1/4-px translations quantize to the nearest 1/4-px phase (10.25, 14.25).
				cmpX, cmpY = 10.25, 14.25
			}
			ttA := NewTester(func(c *Context) {
				Box(c).Fill().Background(bg).Draw(func(p *Painter, _ Rect) {
					p.FillPath(testComplexGlyphPath(cmpX, cmpY), white)
				})
			}, 64, 64)
			ttA.SetScale(scale)
			ttA.Frame()
			imgA := ttA.Image()

			ttB := NewTester(func(c *Context) {
				Box(c).Fill().Background(bg).Draw(func(p *Painter, _ Rect) {
					p.FillBakedPath(baked, ox, oy, white)
				})
			}, 64, 64)
			ttB.SetScale(scale)
			ttB.Frame()
			imgB := ttB.Image()

			for y := 0; y < imgA.Bounds().Dy(); y++ {
				for x := 0; x < imgA.Bounds().Dx(); x++ {
					a, b := imgA.RGBAAt(x, y), imgB.RGBAAt(x, y)
					if a != b {
						t.Fatalf("scale=%v offset=(%v,%v) pixel (%d,%d) baked=%v fill=%v", scale, ox, oy, x, y, b, a)
					}
				}
			}
		}
	}
}

func BenchmarkFillPathVsBaked(b *testing.B) {
	ink := RGB(235, 235, 240)
	bg := RGB(0, 0, 0)

	b.Run("FillPath", func(b *testing.B) {
		var p Path
		step := 0
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Background(bg).Draw(func(pt *Painter, _ Rect) {
				dx := float32(step%4) * 0.25
				for row := range 4 {
					for col := range 8 {
						ox := float32(4+col*20) + dx
						oy := float32(4 + row*28)
						p.Reset()
						p.MoveTo(ox+2, oy+14)
						p.CubeTo(ox+2, oy+5, ox+8, oy+2, ox+14, oy+2)
						p.CubeTo(ox+20, oy+2, ox+24, oy+8, ox+22, oy+14)
						p.QuadTo(ox+18, oy+20, ox+12, oy+22)
						p.CubeTo(ox+6, oy+22, ox+2, oy+19, ox+2, oy+14)
						p.Close()
						p.MoveTo(ox+9, oy+12)
						p.LineTo(ox+15, oy+12)
						p.LineTo(ox+12, oy+8)
						p.Close()
						pt.FillPath(&p, ink)
					}
				}
			})
		}, 200, 140)
		tt.Frame()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			step++
			tt.h.img.Invalidate()
			tt.Frame()
		}
	})

	b.Run("FillBakedPath", func(b *testing.B) {
		baked := BakePath(testComplexGlyphPath(0, 0))
		step := 0
		tt := NewTester(func(c *Context) {
			Box(c).Fill().Background(bg).Draw(func(pt *Painter, _ Rect) {
				dx := float32(step%4) * 0.25
				for row := range 4 {
					for col := range 8 {
						ox := float32(4+col*20) + dx
						oy := float32(4 + row*28)
						pt.FillBakedPath(baked, ox, oy, ink)
					}
				}
			})
		}, 200, 140)
		tt.Frame()
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			step++
			tt.h.img.Invalidate()
			tt.Frame()
		}
	})
}
