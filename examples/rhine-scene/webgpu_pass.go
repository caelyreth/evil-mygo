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
type sceneDepthAttachment struct {
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

type sceneColorAttachment struct {
	nextInChain   uintptr
	view          uintptr
	depthSlice    uint32
	_             uint32
	resolveTarget uintptr
	loadOp        uint32
	storeOp       uint32
	clearValue    wgpu.Color
}

type scenePassDescriptor struct {
	nextInChain            uintptr
	labelData              uintptr
	labelLength            uintptr
	colorAttachmentCount   uintptr
	colorAttachments       *sceneColorAttachment
	depthStencilAttachment *sceneDepthAttachment
	occlusionQuerySet      uintptr
	timestampWrites        uintptr
}

type scenePassAPI struct {
	begin        func(uintptr, *scenePassDescriptor) uintptr
	setPipeline  func(uintptr, uintptr)
	setBindGroup func(uintptr, uint32, uintptr, uintptr, *uint32)
	setIndex     func(uintptr, uintptr, uint32, uint64, uint64)
	draw         func(uintptr, uint32, uint32, uint32, uint32)
	drawIndexed  func(uintptr, uint32, uint32, uint32, int32, uint32)
	end          func(uintptr)
	release      func(uintptr)
}

var loadScenePassAPI = sync.OnceValues(func() (*scenePassAPI, error) {
	path := os.Getenv("WGPU_NATIVE_PATH")
	if path == "" {
		path = "libwgpu_native.dylib"
	}
	library, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return nil, err
	}
	api := &scenePassAPI{}
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

type scenePass struct {
	api    *scenePassAPI
	handle uintptr
}

func beginScenePass(encoder *wgpu.CommandEncoder, color, resolve, depth *wgpu.TextureView, load bool) (*scenePass, error) {
	loadOp := wgpu.LoadOpClear
	if load {
		loadOp = wgpu.LoadOpLoad
	}
	return beginScenePassColor(encoder, color, resolve, depth, load, loadOp, wgpu.Color{A: 1})
}

func beginScenePassColor(encoder *wgpu.CommandEncoder, color, resolve, depth *wgpu.TextureView, loadDepth bool, colorLoad wgpu.LoadOp, clear wgpu.Color) (*scenePass, error) {
	api, err := loadScenePassAPI()
	if err != nil {
		return nil, err
	}
	depthLoad := wgpu.LoadOpClear
	if loadDepth {
		depthLoad = wgpu.LoadOpLoad
	}
	colorAttachment := sceneColorAttachment{depthSlice: wgpu.DepthSliceUndefined,
		loadOp: uint32(colorLoad), storeOp: uint32(wgpu.StoreOpStore), clearValue: clear}
	if color != nil {
		colorAttachment.view = color.Handle()
	}
	if resolve != nil {
		colorAttachment.resolveTarget = resolve.Handle()
	}
	depthAttachment := sceneDepthAttachment{
		view: depth.Handle(), depthLoadOp: uint32(depthLoad),
		depthStoreOp: uint32(wgpu.StoreOpStore), depthClearValue: 1,
	}
	descriptor := scenePassDescriptor{depthStencilAttachment: &depthAttachment}
	if color != nil {
		descriptor.colorAttachmentCount, descriptor.colorAttachments = 1, &colorAttachment
	}
	handle := api.begin(encoder.Handle(), &descriptor)
	if handle == 0 {
		return nil, errors.New("scene depth pass returned a null encoder")
	}
	return &scenePass{api: api, handle: handle}, nil
}

func (p *scenePass) SetPipeline(pipeline *wgpu.RenderPipeline) {
	p.api.setPipeline(p.handle, pipeline.Handle())
}

func (p *scenePass) SetBindGroup(index uint32, bind *wgpu.BindGroup, offsets []uint32) {
	p.api.setBindGroup(p.handle, index, bind.Handle(), uintptr(len(offsets)), unsafe.SliceData(offsets))
}

func (p *scenePass) SetIndexBuffer(buffer *wgpu.Buffer, format wgpu.IndexFormat, offset, size uint64) {
	p.api.setIndex(p.handle, buffer.Handle(), uint32(format), offset, size)
}

func (p *scenePass) Draw(vertices, instances, firstVertex, firstInstance uint32) {
	p.api.draw(p.handle, vertices, instances, firstVertex, firstInstance)
}

func (p *scenePass) DrawIndexed(indices, instances, firstIndex uint32, baseVertex int32, firstInstance uint32) {
	p.api.drawIndexed(p.handle, indices, instances, firstIndex, baseVertex, firstInstance)
}

func (p *scenePass) End() { p.api.end(p.handle) }

func (p *scenePass) Release() {
	if p.handle != 0 {
		p.api.release(p.handle)
		p.handle = 0
	}
}
