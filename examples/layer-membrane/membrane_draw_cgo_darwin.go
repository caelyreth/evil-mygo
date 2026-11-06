//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Foundation -framework Metal
#include "membrane_draw.h"
*/
import "C"

import "github.com/egoist/mygo/ui"

func drawMembrane(frame ui.MetalFrame, camX, camY, camZ, yaw, pitch, waveTime, amplitude, viewAspect float32) error {
	C.mygo_membrane_draw(
		frame.Device, frame.CommandBuffer, frame.Texture, frame.Source,
		C.int(frame.Width), C.int(frame.Height), C.int(frame.SourceWidth), C.int(frame.SourceHeight),
		C.float(camX), C.float(camY), C.float(camZ), C.float(yaw), C.float(pitch),
		C.float(waveTime), C.float(amplitude), C.float(viewAspect),
	)
	return nil
}
