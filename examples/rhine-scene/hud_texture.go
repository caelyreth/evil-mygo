//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import "github.com/caelyreth/evil-mygo/ui"

// Small print textures are native drawing commands, not images or GPU readbacks.
func paintHUDHalftone(p *ui.Painter, r ui.Rect, color ui.Color) {
	const pitch = float32(5)
	for y := pitch / 2; y < r.H-pitch/2; y += pitch {
		for x := pitch / 2; x < r.W-pitch/2; x += pitch {
			falloff := (1 - x/r.W) * (1 - y/r.H)
			radius := .3 + 1.5*falloff
			p.Fill(ui.Rect{X: r.X + x - radius, Y: r.Y + y - radius, W: radius * 2, H: radius * 2}, color, radius)
		}
	}
}

func paintHUDDither(p *ui.Painter, r ui.Rect, color ui.Color) {
	// Ordered thresholds keep the texture stable while the scene animates.
	threshold := [4][4]int{
		{0, 8, 2, 10},
		{12, 4, 14, 6},
		{3, 11, 1, 9},
		{15, 7, 13, 5},
	}
	const pixel = float32(2)
	for row, y := 0, float32(0); y < r.H; row, y = row+1, y+pixel {
		for col, x := 0, float32(0); x < r.W; col, x = col+1, x+pixel {
			density := 1 - x/r.W
			if float32(threshold[row&3][col&3])+.5 >= density*16 {
				continue
			}
			p.Fill(ui.Rect{X: r.X + x, Y: r.Y + y, W: min(pixel, r.W-x), H: min(pixel, r.H-y)}, color, 0)
		}
	}
}
