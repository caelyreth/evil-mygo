package wgsl_test

import (
	"strings"
	"testing"

	"github.com/caelyreth/evil-mygo/internal/shader"
	"github.com/caelyreth/evil-mygo/internal/shaderir/wgsl"
)

func TestFragmentWritesWGSL(t *testing.T) {
	p, err := shader.CompileFragment([]byte(`package main

var Scale float

func Fragment(dst vec4, pos vec2) vec4 {
	return vec4(pos.x*Scale, pos.y, 0, 1)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	src := wgsl.Fragment(p, "shaderFragment")
	for _, want := range []string{
		"fn shaderFragment(position: vec4<f32>, v0: vec2<f32>) -> vec4<f32>",
		"shader_uniforms",
		"return F",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("WGSL output lacks %q:\n%s", want, src)
		}
	}
}

func TestFragmentWritesOutParameters(t *testing.T) {
	p, err := shader.CompileFragment([]byte(`package main

func split(pos vec2) (float, vec2) {
	return pos.x, pos
}

func Fragment(dst vec4, pos vec2) vec4 {
	x, xy := split(pos)
	return vec4(x, xy.x, xy.y, 1)
}
`))
	if err != nil {
		t.Fatal(err)
	}
	src := wgsl.Fragment(p, "shaderFragment")
	for _, want := range []string{
		"ptr<function, f32>",
		"&l",
		"(*l",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("WGSL output lacks %q:\n%s", want, src)
		}
	}
}
