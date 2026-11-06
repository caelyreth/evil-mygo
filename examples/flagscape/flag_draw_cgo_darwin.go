//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "flag_draw.h"
*/
import "C"

import "github.com/egoist/mygo/ui"

func drawPanel(frame ui.MetalFrame, camX, camY, camZ, yaw, pitch float32) error {
	C.mygo_panel_draw(
		frame.Device, frame.CommandBuffer, frame.Texture, frame.Source,
		C.int(frame.Width), C.int(frame.Height), C.int(frame.SourceWidth), C.int(frame.SourceHeight),
		C.float(camX), C.float(camY), C.float(camZ), C.float(yaw), C.float(pitch),
	)
	return nil
}
