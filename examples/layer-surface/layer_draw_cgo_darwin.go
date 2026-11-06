//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "layer_draw.h"
*/
import "C"

import "github.com/egoist/mygo/ui"

func presentLayer(frame ui.MetalFrame) error {
	C.mygo_layer_present(
		frame.Device, frame.CommandBuffer, frame.Texture, frame.Source,
		C.int(frame.Width), C.int(frame.Height), C.int(frame.SourceWidth), C.int(frame.SourceHeight),
	)
	return nil
}
