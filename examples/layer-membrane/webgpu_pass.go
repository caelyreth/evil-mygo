//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	"errors"
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/go-webgpu/webgpu/wgpu"
)

// go-webgpu v0.5.5 omits nextInChain from its depth attachment wire type.
// Keep this v29 ABI workaround local to the example until upstream fixes it.
type silkDepthAttachment struct {
	nextInChain       uintptr
	view              uintptr
	depthLoadOp       uint32
	depthStoreOp      uint32
	depthClearValue   float32
	depthReadOnly     uint32
	stencilLoadOp     uint32
	stencilStoreOp    uint32
	stencilClearValue uint32
	stencilReadOnly   uint32
}

type silkColorAttachment struct {
	nextInChain   uintptr
	view          uintptr
	depthSlice    uint32
	_             uint32
	resolveTarget uintptr
	loadOp        uint32
	storeOp       uint32
	clearValue    wgpu.Color
}

type silkPassDescriptor struct {
	nextInChain            uintptr
	labelData              uintptr
	labelLength            uintptr
	colorAttachmentCount   uintptr
	colorAttachments       *silkColorAttachment
	depthStencilAttachment *silkDepthAttachment
	occlusionQuerySet      uintptr
	timestampWrites        uintptr
}

type silkPassAPI struct {
	begin        func(uintptr, *silkPassDescriptor) uintptr
	setPipeline  func(uintptr, uintptr)
	setBindGroup func(uintptr, uint32, uintptr, uintptr, *uint32)
	setIndex     func(uintptr, uintptr, uint32, uint64, uint64)
	draw         func(uintptr, uint32, uint32, uint32, uint32)
	drawIndexed  func(uintptr, uint32, uint32, uint32, int32, uint32)
	end          func(uintptr)
	release      func(uintptr)
}

var loadSilkPassAPI = sync.OnceValues(func() (*silkPassAPI, error) {
	path := os.Getenv("WGPU_NATIVE_PATH")
	if path == "" {
		path = "libwgpu_native.dylib"
	}
	library, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	api := &silkPassAPI{}
	for name, fn := range map[string]any{
		"wgpuCommandEncoderBeginRenderPass":   &api.begin,
		"wgpuRenderPassEncoderSetPipeline":    &api.setPipeline,
		"wgpuRenderPassEncoderSetBindGroup":   &api.setBindGroup,
		"wgpuRenderPassEncoderSetIndexBuffer": &api.setIndex,
		"wgpuRenderPassEncoderDraw":           &api.draw,
		"wgpuRenderPassEncoderDrawIndexed":    &api.drawIndexed,
		"wgpuRenderPassEncoderEnd":            &api.end,
		"wgpuRenderPassEncoderRelease":        &api.release,
	} {
		address, err := purego.Dlsym(library, name)
		if err != nil {
			purego.Dlclose(library)
			return nil, err
		}
		purego.RegisterFunc(fn, address)
	}
	// Retain the library reference for the lifetime of these cached functions.
	return api, nil
})

type silkPass struct {
	api    *silkPassAPI
	handle uintptr
}

func beginSilkPass(encoder *wgpu.CommandEncoder, color, resolve, depth *wgpu.TextureView) (*silkPass, error) {
	api, err := loadSilkPassAPI()
	if err != nil {
		return nil, err
	}
	colorAttachment := silkColorAttachment{
		view: color.Handle(), resolveTarget: resolve.Handle(), depthSlice: wgpu.DepthSliceUndefined,
		loadOp: uint32(wgpu.LoadOpClear), storeOp: uint32(wgpu.StoreOpDiscard), clearValue: wgpu.Color{A: 1},
	}
	depthAttachment := silkDepthAttachment{
		view: depth.Handle(), depthLoadOp: uint32(wgpu.LoadOpClear),
		depthStoreOp: uint32(wgpu.StoreOpDiscard), depthClearValue: 1,
	}
	descriptor := silkPassDescriptor{
		colorAttachmentCount: 1, colorAttachments: &colorAttachment, depthStencilAttachment: &depthAttachment,
	}
	handle := api.begin(encoder.Handle(), &descriptor)
	if handle == 0 {
		return nil, errors.New("silk depth pass returned a null encoder")
	}
	return &silkPass{api: api, handle: handle}, nil
}

func (p *silkPass) SetPipeline(pipeline *wgpu.RenderPipeline) {
	p.api.setPipeline(p.handle, pipeline.Handle())
}

func (p *silkPass) SetBindGroup(index uint32, bind *wgpu.BindGroup, offsets []uint32) {
	p.api.setBindGroup(p.handle, index, bind.Handle(), uintptr(len(offsets)), unsafe.SliceData(offsets))
}

func (p *silkPass) SetIndexBuffer(buffer *wgpu.Buffer, format wgpu.IndexFormat, offset, size uint64) {
	p.api.setIndex(p.handle, buffer.Handle(), uint32(format), offset, size)
}

func (p *silkPass) Draw(vertices, instances, firstVertex, firstInstance uint32) {
	p.api.draw(p.handle, vertices, instances, firstVertex, firstInstance)
}

func (p *silkPass) DrawIndexed(indices, instances, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.api.drawIndexed(p.handle, indices, instances, firstIndex, baseVertex, firstInstance)
}

func (p *silkPass) End() { p.api.end(p.handle) }

func (p *silkPass) Release() {
	if p.handle != 0 {
		p.api.release(p.handle)
		p.handle = 0
	}
}
