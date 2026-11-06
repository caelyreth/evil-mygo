//go:build darwin && !cgo && !nofakecgo

package webgpu

import (
	"errors"

	"github.com/egoist/mygo/internal/scene"
)

// NativeRenderer is unavailable in the untagged cgo-free build. The
// standalone device and offscreen tests do not link purego; a real native
// surface needs cgo or -tags nofakecgo.
type NativeRenderer struct{}

func NewNative(uintptr) (*NativeRenderer, error) {
	return nil, errors.New("webgpu: native surfaces need cgo or -tags nofakecgo")
}

func (*NativeRenderer) Render(*scene.Scene) error {
	return errors.New("webgpu: native renderer unavailable")
}
func (*NativeRenderer) Invalidate()       {}
func (*NativeRenderer) FrameInfo() string { return "webgpu/unavailable" }
func (*NativeRenderer) Release()          {}
