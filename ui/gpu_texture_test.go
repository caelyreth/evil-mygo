package ui

import "testing"

func TestGPUTextureImplementations(t *testing.T) {
	textures := []GPUTexture{
		NewUILayer(func(c *Context) { Text(c, "texture") }, 8, 4),
	}
	defer textures[0].(*UILayer).Close()
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
