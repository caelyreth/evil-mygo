//go:build darwin && !cgo && (!mygo_webgpu || !nofakecgo)

package ui

import (
	"errors"

	"github.com/caelyreth/evil-mygo/internal/platform"
)

func newGPURenderer(n platform.SurfaceNative) (gpuRenderer, error) {
	return nil, errors.New("native UI on macOS needs cgo, or -tags mygo_webgpu,nofakecgo for the cgo-free WebGPU path")
}
