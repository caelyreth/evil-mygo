//go:build !darwin || !cgo

package main

import "github.com/egoist/mygo/ui"

func drawMembrane(ui.MetalFrame, float32, float32, float32, float32, float32, float32, float32, float32) error {
	return nil
}
