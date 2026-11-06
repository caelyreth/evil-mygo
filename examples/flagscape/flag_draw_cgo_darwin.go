//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "flag_draw.h"
*/
import "C"

import "github.com/egoist/mygo/ui"

func drawFlag(frame ui.MetalFrame, seconds, camX, camY, camZ, yaw, pitch, wind, light float32) error {
	C.mygo_flag_draw(
		frame.Device, frame.CommandBuffer, frame.Texture, frame.Source,
		C.int(frame.Width), C.int(frame.Height), C.int(frame.SourceWidth), C.int(frame.SourceHeight),
		C.float(seconds), C.float(camX), C.float(camY), C.float(camZ),
		C.float(yaw), C.float(pitch), C.float(wind), C.float(light),
	)
	return nil
}
