//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "metal_draw.h"
*/
import "C"

import (
	"github.com/egoist/mygo/ui"
)

func drawMetal(frame ui.MetalFrame, seconds, energy, tint float32) error {
	C.mygo_draw_metal(
		frame.Device,
		frame.CommandBuffer,
		frame.Texture,
		C.int(frame.Width),
		C.int(frame.Height),
		C.float(seconds),
		C.float(energy),
		C.float(tint),
	)
	return nil
}
