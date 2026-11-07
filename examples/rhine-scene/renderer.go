//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	_ "embed"
	"errors"
	"fmt"
	"unsafe"

	"github.com/go-webgpu/webgpu/wgpu"
	"github.com/gogpu/gputypes"

	"github.com/caelyreth/evil-mygo/ui"
)

//go:embed scene.wgsl
var sceneShader string

//go:embed finish.wgsl
var finishShader string

type resource interface{ Release() }
type pipeline struct {
	pipe *wgpu.RenderPipeline
	bind *wgpu.BindGroup
}
type target struct {
	texture *wgpu.Texture
	view    *wgpu.TextureView
}
type sceneRenderer struct {
	model                                                            *model
	instances                                                        []instance
	clock                                                            playbackClock
	controller                                                       *sceneController
	prepared                                                         bool
	preparedAspect                                                   float32
	camera                                                           sceneCamera
	hero                                                             vec3
	glassBeforeHero                                                  int
	placementSize                                                    uint64
	device                                                           *wgpu.Device
	resources                                                        []resource
	targets                                                          []resource
	width, height                                                    int
	uniforms, vertices, indices, placements, materials               *wgpu.Buffer
	sampler                                                          *wgpu.Sampler
	shadow, ground, opaque, dossier, glass, opticalComposite, finish pipeline
	dossierSource                                                    *wgpu.Texture
	dossierView                                                      *wgpu.TextureView
	dossierCell                                                      archiveCell
	dossierVisible                                                   bool
	dossierOpacity                                                   float32
	shadowMap, msaa, depth, capture, optical, final                  target
	draws                                                            uint64
}

func bytesOf[T any](values []T) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(values))), len(values)*int(unsafe.Sizeof(*new(T))))
}

func (r *sceneRenderer) upload(label string, usage wgpu.BufferUsage, data []byte) (*wgpu.Buffer, error) {
	buffer, err := r.device.CreateBuffer(&wgpu.BufferDescriptor{Label: label, Usage: usage | wgpu.BufferUsageCopyDst, Size: uint64(len(data))})
	if err != nil {
		return nil, err
	}
	r.resources = append(r.resources, buffer)
	if err := r.device.Queue().WriteBuffer(buffer, 0, data); err != nil {
		return nil, err
	}
	return buffer, nil
}

func (r *sceneRenderer) init(device *wgpu.Device) (err error) {
	r.device = device
	defer func() {
		if err != nil {
			r.release()
		}
	}()
	module, err := device.CreateShaderModuleWGSL(sceneShader)
	if err != nil {
		return fmt.Errorf("archive shader: %w", err)
	}
	defer module.Release()
	finishModule, err := device.CreateShaderModuleWGSL(finishShader)
	if err != nil {
		return fmt.Errorf("archive finish shader: %w", err)
	}
	defer finishModule.Release()
	desc := wgpu.RenderPipelineDescriptor{
		Label: "Archive shadow", Vertex: wgpu.VertexState{Module: module, EntryPoint: "shadowVertex"},
		// Native TriangleList is 4; go-webgpu v0.5.5 does not convert this enum.
		Primitive:    wgpu.PrimitiveState{Topology: wgpu.PrimitiveTopology(4), CullMode: wgpu.CullModeNone},
		DepthStencil: &wgpu.DepthStencilState{Format: wgpu.TextureFormatDepth32Float, DepthWriteEnabled: true, DepthCompare: wgpu.CompareFunctionLessEqual},
		Multisample:  wgpu.MultisampleState{Count: 1},
	}
	create := func(destination *pipeline) error {
		pipe, err := device.CreateRenderPipeline(&desc)
		if err != nil {
			return fmt.Errorf("%s: %w", desc.Label, err)
		}
		destination.pipe = pipe
		r.resources = append(r.resources, pipe)
		return nil
	}
	if err = create(&r.shadow); err != nil {
		return err
	}
	desc.Label, desc.Vertex.EntryPoint = "Archive ground", "fullscreenVertex"
	desc.Multisample.Count = 4
	desc.Fragment = &wgpu.FragmentState{Module: module, EntryPoint: "groundFragment",
		Targets: []wgpu.ColorTargetState{{Format: wgpu.TextureFormatRGBA16Float, WriteMask: wgpu.ColorWriteMaskAll}}}
	if err = create(&r.ground); err != nil {
		return err
	}
	desc.Label, desc.Vertex.EntryPoint, desc.Fragment.EntryPoint = "Archive physical surfaces", "meshVertex", "opaqueFragment"
	if err = create(&r.opaque); err != nil {
		return err
	}
	desc.Label, desc.Fragment.EntryPoint = "Archive native dossier", "dossierFragment"
	if err = create(&r.dossier); err != nil {
		return err
	}
	desc.Label, desc.Fragment.EntryPoint = "Archive optical cover", "glassFragment"
	desc.DepthStencil.DepthWriteEnabled = false
	// Add optical contributions rather than overwriting another plastic sheet.
	// This remains instanced and independent of mesh/instance submission order.
	desc.Fragment.Targets[0].Blend = &wgpu.BlendState{
		Color: wgpu.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne},
		Alpha: wgpu.BlendComponent{Operation: gputypes.BlendOperationAdd, SrcFactor: gputypes.BlendFactorOne, DstFactor: gputypes.BlendFactorOne},
	}
	if err = create(&r.glass); err != nil {
		return err
	}
	desc.Label = "Archive layered plastic composition"
	desc.Vertex = wgpu.VertexState{Module: module, EntryPoint: "fullscreenVertex"}
	desc.Fragment = &wgpu.FragmentState{Module: module, EntryPoint: "opticalCompositeFragment",
		Targets: []wgpu.ColorTargetState{{Format: wgpu.TextureFormatRGBA16Float, WriteMask: wgpu.ColorWriteMaskAll}}}
	desc.DepthStencil, desc.Multisample.Count = nil, 1
	if err = create(&r.opticalComposite); err != nil {
		return err
	}
	desc.Label = "Archive lens and color output"
	desc.Vertex = wgpu.VertexState{Module: finishModule, EntryPoint: "vertexMain"}
	desc.Fragment = &wgpu.FragmentState{Module: finishModule, EntryPoint: "fragmentMain",
		Targets: []wgpu.ColorTargetState{{Format: wgpu.TextureFormatRGBA8Unorm, WriteMask: wgpu.ColorWriteMaskAll}}}
	desc.DepthStencil, desc.Multisample.Count = nil, 1
	if err = create(&r.finish); err != nil {
		return err
	}
	r.uniforms, err = r.upload("Archive camera", wgpu.BufferUsageUniform, bytesOf([]globals{{}}))
	if err != nil {
		return err
	}
	// Vertex pulling avoids the pinned wrapper's incompatible vertex-layout ABI.
	r.vertices, err = r.upload("Archive geometry", wgpu.BufferUsageStorage, bytesOf(r.model.vertices))
	if err != nil {
		return err
	}
	r.indices, err = r.upload("Archive triangles", wgpu.BufferUsageIndex, bytesOf(r.model.indices))
	if err != nil {
		return err
	}
	// Visibility changes the live count, not the buffer or its bind groups.
	placements := make([]instance, residentCapacity)
	copy(placements, r.instances)
	r.placementSize = uint64(len(bytesOf(placements)))
	r.placements, err = r.upload("Archive instances", wgpu.BufferUsageStorage, bytesOf(placements))
	if err != nil {
		return err
	}
	r.materials, err = r.upload("Archive materials", wgpu.BufferUsageStorage, bytesOf(r.model.materials))
	if err != nil {
		return err
	}
	r.sampler, err = device.CreateSampler(&wgpu.SamplerDescriptor{
		Label: "Archive linear capture", AddressModeU: wgpu.AddressModeClampToEdge, AddressModeV: wgpu.AddressModeClampToEdge, AddressModeW: wgpu.AddressModeClampToEdge,
		MinFilter: wgpu.FilterModeLinear, MagFilter: wgpu.FilterModeLinear, MipmapFilter: gputypes.MipmapFilterModeLinear, LodMaxClamp: 1,
	})
	if err == nil {
		r.resources = append(r.resources, r.sampler)
	}
	return err
}

func (r *sceneRenderer) newTarget(label string, width, height int, format wgpu.TextureFormat, samples uint32, sample bool) (target, error) {
	usage := wgpu.TextureUsageRenderAttachment
	if sample {
		usage |= wgpu.TextureUsageTextureBinding
	}
	texture, err := r.device.CreateTexture(&wgpu.TextureDescriptor{Label: label, Usage: usage,
		Size: wgpu.Extent3D{Width: uint32(width), Height: uint32(height), DepthOrArrayLayers: 1}, Format: format, SampleCount: samples, MipLevelCount: 1})
	if err != nil {
		return target{}, err
	}
	r.targets = append(r.targets, texture)
	view, err := texture.CreateView(nil)
	if err != nil {
		return target{}, err
	}
	r.targets = append(r.targets, view)
	return target{texture, view}, nil
}

func (r *sceneRenderer) resize(width, height int) (err error) {
	if r.width == width && r.height == height && r.finish.bind != nil {
		return nil
	}
	r.releaseTargets()
	defer func() {
		if err != nil {
			r.releaseTargets()
		}
	}()
	r.shadowMap, err = r.newTarget("Archive soft shadows", shadowMapSize, shadowMapSize, wgpu.TextureFormatDepth32Float, 1, true)
	if err != nil {
		return err
	}
	r.msaa, err = r.newTarget("Archive shared HDR/optical multisample scratch", width, height, wgpu.TextureFormatRGBA16Float, 4, false)
	if err != nil {
		return err
	}
	r.depth, err = r.newTarget("Archive multisample depth", width, height, wgpu.TextureFormatDepth32Float, 4, true)
	if err != nil {
		return err
	}
	r.capture, err = r.newTarget("Archive opaque transmission capture", width, height, wgpu.TextureFormatRGBA16Float, 1, true)
	if err != nil {
		return err
	}
	r.optical, err = r.newTarget("Archive layered plastic accumulation", width, height, wgpu.TextureFormatRGBA16Float, 1, true)
	if err != nil {
		return err
	}
	r.final, err = r.newTarget("Archive HDR composite", width, height, wgpu.TextureFormatRGBA16Float, 1, true)
	if err != nil {
		return err
	}
	entries := map[uint32]wgpu.BindGroupEntry{
		0: wgpu.BufferBindingEntry(0, r.uniforms, 0, uint64(unsafe.Sizeof(globals{}))),
		1: wgpu.BufferBindingEntry(1, r.vertices, 0, uint64(len(bytesOf(r.model.vertices)))),
		2: wgpu.BufferBindingEntry(2, r.placements, 0, r.placementSize),
		3: wgpu.BufferBindingEntry(3, r.materials, 0, uint64(len(bytesOf(r.model.materials)))),
		4: wgpu.TextureBindingEntry(4, r.shadowMap.view),
		5: wgpu.TextureBindingEntry(5, r.capture.view),
		6: wgpu.SamplerBindingEntry(6, r.sampler),
		7: wgpu.TextureBindingEntry(7, r.optical.view),
	}
	for _, group := range []struct {
		p        *pipeline
		bindings []uint32
	}{
		{&r.shadow, []uint32{0, 1, 2}}, {&r.ground, []uint32{0, 4}},
		{&r.opaque, []uint32{0, 1, 2, 3, 4}}, {&r.glass, []uint32{0, 1, 2, 3, 5, 6}},
		{&r.opticalComposite, []uint32{0, 5, 6, 7}},
	} {
		var bindings []wgpu.BindGroupEntry
		for _, binding := range group.bindings {
			bindings = append(bindings, entries[binding])
		}
		if err = r.bind(group.p, bindings); err != nil {
			return err
		}
	}
	if err = r.bind(&r.finish, []wgpu.BindGroupEntry{entries[0], wgpu.TextureBindingEntry(1, r.final.view), wgpu.TextureBindingEntry(2, r.depth.view), wgpu.SamplerBindingEntry(3, r.sampler)}); err != nil {
		return err
	}
	r.width, r.height = width, height
	return nil
}

func (r *sceneRenderer) bind(p *pipeline, entries []wgpu.BindGroupEntry) error {
	layout := p.pipe.GetBindGroupLayout(0)
	if layout == nil {
		return errors.New("archive pipeline has no bind group layout")
	}
	defer layout.Release()
	bind, err := r.device.CreateBindGroupSimple(layout, entries)
	if err != nil {
		return err
	}
	p.bind = bind
	r.targets = append(r.targets, bind)
	return nil
}

func (r *sceneRenderer) draw(frame ui.WebGPUFrame) error {
	if frame.Device == nil || frame.Encoder == nil || frame.Texture == nil || frame.Width <= 0 || frame.Height <= 0 {
		return nil
	}
	if r.device != nil && r.device != frame.Device {
		r.release()
	}
	aspect := float32(frame.Width) / float32(frame.Height)
	if !r.prepared || r.preparedAspect != aspect {
		r.prepare(aspect)
	}
	r.prepared = false
	shot, camera, hero := sceneTime(r.clock.elapsed), r.camera, r.hero
	if r.device == nil {
		if err := r.init(frame.Device); err != nil {
			return err
		}
	}
	if err := r.resize(frame.Width, frame.Height); err != nil {
		return err
	}
	if err := r.bindDossier(frame); err != nil {
		return err
	}
	values := sceneGlobalsFor(frame.Width, frame.Height, hero, camera)
	values.pages[0] = r.dossierOpacity
	if r.controller != nil {
		values.underlight = r.controller.selectionLamp()
		values.light[3] = float32(r.controller.night.value)
		if r.controller.interactive {
			values.size[2] = float32(r.controller.focus.value)
		}
	} else {
		values.underlight = entranceLamp(shot, hero)
	}
	if len(r.instances) > 0 {
		if err := r.device.Queue().WriteBuffer(r.placements, 0, bytesOf(r.instances)); err != nil {
			return err
		}
	}
	if err := r.device.Queue().WriteBuffer(r.uniforms, 0, bytesOf([]globals{values})); err != nil {
		return err
	}
	shadow, err := beginScenePass(frame.Encoder, nil, nil, r.shadowMap.view, false)
	if err != nil {
		return err
	}
	r.setMeshPass(shadow, &r.shadow)
	for _, mesh := range r.model.meshes {
		if mesh.name == "Optical_Diffuser" {
			shadow.DrawIndexed(mesh.count, uint32(len(r.instances)), mesh.first, 0, 0)
		}
	}
	shadow.End()
	shadow.Release()
	opaque, err := beginScenePass(frame.Encoder, r.msaa.view, r.capture.view, r.depth.view, false)
	if err != nil {
		return err
	}
	opaque.SetPipeline(r.ground.pipe)
	opaque.SetBindGroup(0, r.ground.bind, nil)
	opaque.Draw(3, 1, 0, 0)
	r.setMeshPass(opaque, &r.opaque)
	r.drawMeshes(opaque, false)
	if frame.SourceTexture != nil && r.dossier.bind != nil && r.dossierVisible {
		dossierPosition := hero
		if r.controller != nil {
			pose := r.controller.card(r.dossierCell).position
			dossierPosition = vec3{pose[0], pose[1], pose[2]}
		}
		r.setMeshPass(opaque, &r.dossier)
		for _, mesh := range r.model.meshes {
			if mesh.name != "Optical_Diffuser" {
				continue
			}
			for i, placement := range r.instances {
				if placement.position[0] == dossierPosition.x && placement.position[2] == dossierPosition.z {
					opaque.DrawIndexed(mesh.count, 1, mesh.first, 0, uint32(i))
					break
				}
			}
		}
	}
	opaque.End()
	opaque.Release()
	// The opaque capture is already resolved; reuse its multisample scratch.
	glass, err := beginScenePassColor(frame.Encoder, r.msaa.view, r.optical.view, r.depth.view, true, wgpu.LoadOpClear, wgpu.Color{})
	if err != nil {
		return err
	}
	r.setMeshPass(glass, &r.glass)
	r.drawMeshes(glass, true)
	glass.End()
	glass.Release()
	composite, err := frame.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{Label: "Archive frosted sandwich composition",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{View: r.final.view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore, ClearValue: wgpu.Color{A: 1}}}})
	if err != nil {
		return err
	}
	composite.SetPipeline(r.opticalComposite.pipe)
	composite.SetBindGroup(0, r.opticalComposite.bind, nil)
	composite.Draw(3, 1, 0, 0)
	composite.End()
	composite.Release()
	view, err := frame.Texture.CreateView(nil)
	if err != nil {
		return err
	}
	defer view.Release()
	finish, err := frame.Encoder.BeginRenderPass(&wgpu.RenderPassDescriptor{Label: "Archive final output",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{View: view, LoadOp: wgpu.LoadOpClear, StoreOp: wgpu.StoreOpStore, ClearValue: wgpu.Color{A: 1}}}})
	if err != nil {
		return err
	}
	finish.SetPipeline(r.finish.pipe)
	finish.SetBindGroup(0, r.finish.bind, nil)
	finish.Draw(3, 1, 0, 0)
	finish.End()
	finish.Release()
	r.draws++
	return nil
}

// The host prepares this pose before binding its SurfaceMap. Draw consumes
// exactly that pose, so hit testing, IME and the texture share one snapshot.
func (r *sceneRenderer) prepare(aspect float32) {
	shot := sceneTime(r.clock.elapsed)
	r.camera, r.hero = cameraAt(shot, aspect), heroPosition(shot)
	if r.controller != nil {
		r.instances, r.camera, r.hero = r.controller.snapshot(r.instances, aspect)
	} else {
		r.instances = sceneInstances(r.instances, shot, r.camera.forward)
		r.glassBeforeHero = glassSplit(r.instances, r.camera.forward)
	}
	r.prepared, r.preparedAspect = true, aspect
}

func (r *sceneRenderer) bindDossier(frame ui.WebGPUFrame) error {
	if frame.SourceTexture == nil || r.dossierSource == frame.SourceTexture && r.dossier.bind != nil {
		return nil
	}
	r.releaseDossier()
	view, err := frame.SourceTexture.CreateView(nil)
	if err != nil {
		return err
	}
	layout := r.dossier.pipe.GetBindGroupLayout(0)
	if layout == nil {
		view.Release()
		return errors.New("archive native dossier has no bind group layout")
	}
	defer layout.Release()
	bind, err := r.device.CreateBindGroupSimple(layout, []wgpu.BindGroupEntry{
		wgpu.BufferBindingEntry(0, r.uniforms, 0, uint64(unsafe.Sizeof(globals{}))),
		wgpu.BufferBindingEntry(1, r.vertices, 0, uint64(len(bytesOf(r.model.vertices)))),
		wgpu.BufferBindingEntry(2, r.placements, 0, r.placementSize),
		wgpu.BufferBindingEntry(3, r.materials, 0, uint64(len(bytesOf(r.model.materials)))),
		wgpu.TextureBindingEntry(4, r.shadowMap.view),
		wgpu.SamplerBindingEntry(6, r.sampler),
		wgpu.TextureBindingEntry(8, view),
	})
	if err != nil {
		view.Release()
		return err
	}
	r.dossier.bind, r.dossierView, r.dossierSource = bind, view, frame.SourceTexture
	return nil
}

func (r *sceneRenderer) releaseDossier() {
	if r.dossier.bind != nil {
		r.dossier.bind.Release()
		r.dossier.bind = nil
	}
	if r.dossierView != nil {
		r.dossierView.Release()
		r.dossierView = nil
	}
	r.dossierSource = nil
}

func (r *sceneRenderer) setMeshPass(pass *scenePass, p *pipeline) {
	pass.SetPipeline(p.pipe)
	pass.SetBindGroup(0, p.bind, nil)
	pass.SetIndexBuffer(r.indices, wgpu.IndexFormatUint32, 0, uint64(len(bytesOf(r.model.indices))))
}

func (r *sceneRenderer) drawMeshes(pass *scenePass, glass bool) {
	for _, mesh := range r.model.meshes {
		if mesh.glass != glass {
			continue
		}
		if r.controller != nil {
			if mesh.array {
				pass.DrawIndexed(mesh.count, uint32(len(r.instances)), mesh.first, 0, 0)
			} else {
				for i, placement := range r.instances {
					if placement.position[3] > .5 {
						pass.DrawIndexed(mesh.count, 1, mesh.first, 0, uint32(i))
					}
				}
			}
			continue
		}
		if mesh.array {
			if glass {
				before, count := r.glassBeforeHero, len(r.instances)-1
				if before > 0 {
					pass.DrawIndexed(mesh.count, uint32(before), mesh.first, 0, 0)
				}
				pass.DrawIndexed(mesh.count, 1, mesh.first, 0, uint32(count))
				if before < count {
					pass.DrawIndexed(mesh.count, uint32(count-before), mesh.first, 0, uint32(before))
				}
				continue
			}
			pass.DrawIndexed(mesh.count, uint32(len(r.instances)-1), mesh.first, 0, 0)
		}
		pass.DrawIndexed(mesh.count, 1, mesh.first, 0, uint32(len(r.instances)-1))
	}
}

func (r *sceneRenderer) releaseTargets() {
	r.releaseDossier()
	for i := len(r.targets) - 1; i >= 0; i-- {
		r.targets[i].Release()
	}
	r.targets = nil
	r.width, r.height = 0, 0
	r.shadow.bind, r.ground.bind, r.opaque.bind, r.glass.bind, r.opticalComposite.bind, r.finish.bind = nil, nil, nil, nil, nil, nil
}

func (r *sceneRenderer) release() {
	r.releaseTargets()
	for i := len(r.resources) - 1; i >= 0; i-- {
		r.resources[i].Release()
	}
	r.resources, r.device = nil, nil
}
