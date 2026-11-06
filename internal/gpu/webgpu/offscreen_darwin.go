//go:build darwin

package webgpu

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
)

// RenderOffscreen draws s into a BGRA8 texture and returns its rows in
// MyGo's premultiplied BGRA layout. It is intended for renderer tests and
// comparisons with the CPU rasterizer; it waits for completion.
func RenderOffscreen(s *scene.Scene) ([]byte, error) {
	if s == nil || s.Width <= 0 || s.Height <= 0 {
		return nil, fmt.Errorf("webgpu: invalid offscreen scene size")
	}
	g, err := Open()
	if err != nil {
		return nil, err
	}
	defer g.Release()
	target, err := g.Device().CreateTexture(&wgpu.TextureDescriptor{
		Usage:         wgpu.TextureUsageRenderAttachment | wgpu.TextureUsageCopySrc,
		Size:          wgpu.Extent3D{Width: uint32(s.Width), Height: uint32(s.Height), DepthOrArrayLayers: 1},
		Format:        wgpu.TextureFormatRGBA8Unorm,
		MipLevelCount: 1,
		SampleCount:   1,
	})
	if err != nil {
		return nil, err
	}
	defer target.Release()
	view, err := target.CreateView(nil)
	if err != nil {
		return nil, err
	}
	defer view.Release()
	r, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		return nil, err
	}
	defer r.Release()
	encoder, err := g.Device().CreateCommandEncoder(nil)
	if err != nil {
		return nil, err
	}
	defer encoder.Release()
	if err := r.RenderTarget(s, encoder, target, view); err != nil {
		return nil, err
	}
	rowBytes := (s.Width*4 + 255) &^ 255
	size := uint64(rowBytes * s.Height)
	readback, err := g.Device().CreateBuffer(&wgpu.BufferDescriptor{
		Usage: wgpu.BufferUsageMapRead | wgpu.BufferUsageCopyDst,
		Size:  size,
	})
	if err != nil {
		return nil, err
	}
	defer readback.Release()
	encoder.CopyTextureToBuffer(target, readback, []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: target, Aspect: wgpu.TextureAspectAll},
		BufferLayout: wgpu.ImageDataLayout{
			BytesPerRow: uint32(rowBytes), RowsPerImage: uint32(s.Height),
		},
		Size: wgpu.Extent3D{Width: uint32(s.Width), Height: uint32(s.Height), DepthOrArrayLayers: 1},
	}})
	command, err := encoder.Finish(nil)
	if err != nil {
		return nil, err
	}
	defer command.Release()
	if _, err := g.Queue().Submit(command); err != nil {
		return nil, err
	}
	r.FinishFrame()
	if err := readback.Map(context.Background(), wgpu.MapModeRead, 0, size); err != nil {
		return nil, err
	}
	defer readback.Unmap()
	ptr := readback.GetMappedRange(0, size)
	if ptr == nil {
		return nil, fmt.Errorf("webgpu: mapped readback has no bytes")
	}
	src := unsafe.Slice((*byte)(ptr), int(size))
	dst := make([]byte, 4*s.Width*s.Height)
	for y := range s.Height {
		copy(dst[y*s.Width*4:(y+1)*s.Width*4], src[y*rowBytes:y*rowBytes+s.Width*4])
	}
	for i := 0; i < len(dst); i += 4 {
		dst[i], dst[i+2] = dst[i+2], dst[i]
	}
	return dst, nil
}
