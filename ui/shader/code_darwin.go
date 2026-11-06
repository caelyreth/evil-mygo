package shader

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/internal/gpu/metal"
	"github.com/egoist/mygo/internal/scene"
	internalshader "github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/msl"
	"github.com/egoist/mygo/internal/shaderir/wgsl"
)

// code gives fx p's code for the Metal renderer, and returns where the
// uniforms' lanes lie in its buffer and the buffer's size: the whole
// library, whose tail (ShaderSource in package metal) calls shaderMain.
// It starts making the library and its pipeline, which take Metal 0.1 s
// each the first time, on a goroutine of its own, which the renderers
// then share.
func code(p *shaderir.Program, fx *scene.Effect) ([]int, int) {
	fx.Metal.Source = metal.ShaderSource(msl.Fragment(p, "shaderFragment") + `
float4 shaderMain(constant Uniforms& uniforms, texture2d<float> T0, texture2d<float> T1, texture2d<float> T2, texture2d<float> T3, float2 p, float4 rect) {
	return shaderFragment(uniforms, T0, T1, T2, T3, float4(p, 0.0f, 1.0f), (p - rect.xy) / uniforms.U2);
}
`)
	metal.PrecompileShader(fx.Metal.Source)
	uniformOffsets, _ := msl.UniformLayout(p)
	scaleOffset := 0
	for i, name := range p.UniformNames {
		if name == internalshader.ScaleUniform {
			scaleOffset = uniformOffsets[i]
			break
		}
	}
	args := []string{"vec4<f32>(p, 0.0, 1.0)"}
	for range p.Varyings {
		args = append(args, "(p - e.rect.xy) / max(bitcast<f32>(shader_uniforms["+fmt.Sprint(scaleOffset/4)+"]), 1e-6)")
	}
	fx.WGSL = wgsl.Fragment(p, "shaderFragment") + fmt.Sprintf(`
fn effect(p: vec2<f32>, e: Effect) -> vec4<f32> {
	return shaderFragment(%s);
}
`, strings.Join(args, ", "))
	return msl.UniformLanes(p)
}
