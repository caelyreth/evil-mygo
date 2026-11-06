//go:build darwin

package webgpu

import (
	"math"
	"testing"

	"github.com/go-webgpu/webgpu/wgpu"

	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir/wgsl"
)

func TestEffectAcceptsGeneratedShaderLibrary(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	p, err := shader.CompileFragment([]byte(`package main

var Scale float

func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(pos.x*Scale, pos.y, 0, 1)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	fx := &scene.Effect{
		Name: "generated",
		WGSL: wgsl.Fragment(p, "shaderFragment") + `
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	return shaderFragment(vec4<f32>(p, 0.0, 1.0), (p - e.rect.xy) / max(e.size.z, 1.0));
}
`,
	}
	g, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	defer g.Release()
	r, err := NewRenderer(g, wgpu.TextureFormatRGBA8Unorm)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Release()
	if _, err := r.effectPipelineFor(fx); err != nil {
		t.Fatal(err)
	}
}

func TestEffectReadsStorageUniforms(t *testing.T) {
	if _, ok := RuntimePath(); !ok {
		t.Skip("wgpu-native is not installed")
	}
	lanes := make([]int32, 27)
	copy(lanes, []int32{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 48, 52, 64, 68, 72, 80, 84, 88, 96, 100, 104, 108, 112, 116, 120, 124})
	fx := &scene.Effect{
		Name: "storage uniform",
		WGSL: `
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	return vec4<f32>(bitcast<f32>(shader_uniforms[28]), 0.0, 0.0, 1.0);
}`,
		Shader: &scene.ShaderLayout{Lanes: lanes, Size: 128},
	}
	words := make([]uint32, 27)
	words[23] = math.Float32bits(1)
	s := &scene.Scene{
		Width: 16, Height: 16,
		Ops:     []scene.Op{{Kind: scene.OpEffect, Rect: scene.Rect{W: 16, H: 16}}},
		Effects: []scene.EffectOp{{Effect: fx, Uniforms: words}},
	}
	pix, err := RenderOffscreen(s)
	if err != nil {
		t.Fatal(err)
	}
	p := pix[(8*16+8)*4:][:4]
	if p[0] != 0 || p[1] != 0 || p[2] != 255 || p[3] != 255 {
		t.Fatalf("storage uniform pixel = %v, want red BGRA", p)
	}
}
