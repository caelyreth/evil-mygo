// Package webgpu owns the framework's wgpu-native instance, adapter and
// device. It is the only new GPU API boundary: platform renderers select
// and size a target, while application graphics use WebGPU resources.
package webgpu

import (
	"errors"
	"fmt"

	"github.com/go-webgpu/webgpu/wgpu"
)

// GPU owns a wgpu-native instance, its preferred hardware adapter, the
// logical device and the device's default queue.
type GPU struct {
	instance *wgpu.Instance
	adapter  *wgpu.Adapter
	device   *wgpu.Device
	queue    *wgpu.Queue
	info     wgpu.AdapterInfoGo
}

// Open initializes wgpu-native and requests a high-performance adapter.
// The first call also locates a development lib/wgpu-native library when
// WGPU_NATIVE_PATH is unset.
func Open() (*GPU, error) {
	if err := prepareRuntime(); err != nil {
		return nil, err
	}
	if err := wgpu.Init(); err != nil {
		return nil, fmt.Errorf("webgpu: initialize wgpu-native: %w", err)
	}
	instance, err := wgpu.CreateInstance(nil)
	if err != nil {
		return nil, fmt.Errorf("webgpu: create instance: %w", err)
	}
	g := &GPU{instance: instance}
	fail := func(err error) (*GPU, error) {
		g.Release()
		return nil, err
	}
	g.adapter, err = instance.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: wgpu.PowerPreferenceHighPerformance,
	})
	if err != nil {
		return fail(fmt.Errorf("webgpu: request adapter: %w", err))
	}
	if info, err := g.adapter.Info(); err == nil && info != nil {
		g.info = *info
	}
	g.device, err = g.adapter.RequestDevice(nil)
	if err != nil {
		return fail(fmt.Errorf("webgpu: request device: %w", err))
	}
	g.queue = g.device.Queue()
	if g.queue == nil {
		return fail(errors.New("webgpu: device has no default queue"))
	}
	return g, nil
}

// Device returns the logical WebGPU device.
func (g *GPU) Device() *wgpu.Device {
	if g == nil {
		return nil
	}
	return g.device
}

// Instance returns the wgpu-native instance, for asynchronous error scopes.
func (g *GPU) Instance() *wgpu.Instance {
	if g == nil {
		return nil
	}
	return g.instance
}

// Queue returns the device's default queue.
func (g *GPU) Queue() *wgpu.Queue {
	if g == nil {
		return nil
	}
	return g.queue
}

// Adapter returns the selected adapter.
func (g *GPU) Adapter() *wgpu.Adapter {
	if g == nil {
		return nil
	}
	return g.adapter
}

// AdapterInfo returns the selected adapter's descriptive information.
func (g *GPU) AdapterInfo() wgpu.AdapterInfoGo {
	if g == nil {
		return wgpu.AdapterInfoGo{}
	}
	return g.info
}

// Release frees the queue, device, adapter and instance. It is safe to
// call more than once.
func (g *GPU) Release() {
	if g == nil {
		return
	}
	if g.queue != nil {
		g.queue.Release()
		g.queue = nil
	}
	if g.device != nil {
		g.device.Release()
		g.device = nil
	}
	if g.adapter != nil {
		g.adapter.Release()
		g.adapter = nil
	}
	if g.instance != nil {
		g.instance.Release()
		g.instance = nil
	}
}
