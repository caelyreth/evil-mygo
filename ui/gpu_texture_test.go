package ui

import "testing"

func TestGPUTextureImplementations(t *testing.T) {
	textures := []GPUTexture{
		NewMetalCanvas(8, 4, nil),
		NewMetalTexture(8, 4, nil, nil, nil),
		NewUILayer(func(c *Context) { Text(c, "texture") }, 8, 4),
	}
	defer textures[2].(*UILayer).Close()
	for _, tex := range textures {
		if w, h := tex.Size(); w != 8 || h != 4 {
			t.Fatalf("%T size = %d×%d", tex, w, h)
		}
		if tex.Bitmap() == nil {
			t.Fatalf("%T has no bitmap", tex)
		}
		tex.Invalidate()
	}
}
