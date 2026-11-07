//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package shader

import (
	"math"
	"os"
	"testing"

	"github.com/caelyreth/evil-mygo/internal/gpu/gputest"
	"github.com/caelyreth/evil-mygo/internal/gpu/webgpu"
	"github.com/caelyreth/evil-mygo/internal/scene"
	"github.com/caelyreth/evil-mygo/internal/shaderir"
)

func TestWebGPUShader(t *testing.T) {
	if os.Getenv("MYGO_GPU_STRICT") == "" {
		t.Skip("WebGPU shader parity is being tightened; set MYGO_GPU_STRICT=1 to compare pixels")
	}
	s := MustCompile(testSource)
	sc := testScene(s)
	pix, err := webgpu.RenderOffscreen(sc)
	if err != nil {
		t.Fatal(err)
	}
	gputest.Compare(t, "ui-shader-webgpu", pix, sc.Width*4, sc)
}

func TestWebGPUShaderBools(t *testing.T) {
	sc := boolScene()
	pix, err := webgpu.RenderOffscreen(sc)
	if err != nil {
		t.Fatal(err)
	}
	gputest.Compare(t, "ui-shader-webgpu-bools", pix, sc.Width*4, sc)
}

func TestWebGPUShaderDfdy(t *testing.T) {
	pix, err := webgpu.RenderOffscreen(dfdyScene())
	if err != nil {
		t.Fatal(err)
	}
	checkDfdy(t, pix, 40*4)
}

func TestWebGPUShaderMasked(t *testing.T) {
	sc := maskedScene(MustCompile(maskedSource))
	pix, err := webgpu.RenderOffscreen(sc)
	if err != nil {
		t.Fatal(err)
	}
	gputest.Compare(t, "ui-shader-webgpu-masked", pix, sc.Width*4, sc)
}

func TestWebGPUShaderEffect(t *testing.T) {
	for name, sc := range effectScenes(MustCompile(effectSource)) {
		pix, err := webgpu.RenderOffscreen(sc)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(pix) != 4*sc.Width*sc.Height {
			t.Fatalf("%s: rendered %d bytes", name, len(pix))
		}
	}
}

func TestWebGPUShaderUniform(t *testing.T) {
	s := MustCompile(`package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	return Tint
}
`)
	words := make([]uint32, s.words)
	words[scaleWord] = math.Float32bits(1)
	set(words[s.uniforms["Tint"].word:], shaderir.Float, "Tint", []float32{1, 0, 0, 1})
	s.fx.Shader = &scene.ShaderLayout{
		Lanes: []int32{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 48, 52, 64, 68, 72, 80, 84, 88, 96, 100, 104, 108, 112, 116, 120, 124},
		Size:  128,
	}
	sc := &scene.Scene{Width: 16, Height: 16, Clear: scene.Color{A: 255}}
	sc.Ops = []scene.Op{{Kind: scene.OpEffect, Rect: scene.Rect{W: 16, H: 16}}}
	sc.Effects = []scene.EffectOp{{Effect: s.fx, Uniforms: words}}
	pix, err := webgpu.RenderOffscreen(sc)
	if err != nil {
		t.Fatal(err)
	}
	p := pix[(8*16+8)*4:][:4]
	if p[0] != 0 || p[1] != 0 || p[2] != 255 || p[3] != 255 {
		t.Fatalf("uniform pixel = %v, want red BGRA", p)
	}
}
