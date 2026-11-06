//go:build !darwin || !cgo

package main

import "github.com/egoist/mygo/ui"

func presentLayer(ui.MetalFrame) error { return nil }
