package webgpu

import _ "embed"

// sceneShader is the WGSL port of MyGo's native scene shader. It reads the
// same fourteen-float4 gpu.Instance stream as the other GPU renderers.
//
//go:embed scene.wgsl
var sceneShader string
