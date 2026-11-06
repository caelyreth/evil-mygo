//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "layer_draw.h"
*/
import "C"

import (
	"github.com/egoist/mygo/ui"
)

func drawLayer(frame ui.MetalFrame, seconds, energy float32) error {
	C.mygo_draw_layer(
		frame.Device,
		frame.CommandBuffer,
		frame.Texture,
		frame.Source,
		C.int(frame.Width),
		C.int(frame.Height),
		C.int(frame.SourceWidth),
		C.int(frame.SourceHeight),
		C.float(seconds),
		C.float(energy),
	)
	return nil
}
