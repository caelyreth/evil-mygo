package shader

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/internal/scene"
	internalshader "github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/packed"
	"github.com/egoist/mygo/internal/shaderir/wgsl"
)

// code gives fx p's WGSL code for the WebGPU renderer, and returns where
// the uniforms' lanes lie in its byte-addressable buffer and the buffer's
// size.
func code(p *shaderir.Program, fx *scene.Effect) ([]int, int) {
	uniformOffsets, _ := packed.UniformLayout(p)
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
	return packed.UniformLanes(p)
}
