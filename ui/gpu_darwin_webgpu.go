//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package ui

import (
	"errors"

	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/internal/platform"
)

func newGPURenderer(n platform.SurfaceNative) (gpuRenderer, error) {
	if n.View == 0 && n.Layer == 0 {
		return nil, errors.New("the surface has no view or layer")
	}
	if n.View != 0 {
		return webgpu.NewNative(n.View)
	}
	return webgpu.NewNative(n.Layer)
}
