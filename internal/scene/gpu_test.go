package scene

import "testing"

func TestEachGPUCanvasFindsNestedImages(t *testing.T) {
	gpu := NewGPUCanvas(8, 6, func(GPUFrame) error { return nil })
	outer := NewCanvas(32, 24)
	outer.Scene.Ops = append(outer.Scene.Ops, Op{Kind: OpImage, Image: gpu})
	top := &Scene{}
	top.Ops = append(top.Ops, Op{Kind: OpImage, Image: outer})

	var canvases, gpuCanvases int
	top.EachCanvas(func(*Image) { canvases++ })
	top.EachGPUCanvas(func(img *Image) {
		gpuCanvases++
		if img != gpu {
			t.Fatalf("found %p, want %p", img, gpu)
		}
	})
	if canvases != 1 || gpuCanvases != 1 {
		t.Fatalf("found %d canvases and %d GPU canvases, want 1 and 1", canvases, gpuCanvases)
	}
}

func TestGPUCanvasResizeKeepsNoPixels(t *testing.T) {
	img := NewGPUCanvas(4, 3, nil)
	img.Resize(9, 7)
	if img.W != 9 || img.H != 7 || img.Pixels() != nil {
		t.Fatalf("resized GPU canvas is %d×%d with pixels %v", img.W, img.H, img.Pixels() != nil)
	}
}

func TestExternalImageHasNoPixels(t *testing.T) {
	img := NewExternalImage(4, 3, nil, nil, "owner")
	if img.W != 4 || img.H != 3 || img.Pixels() != nil || img.External == nil {
		t.Fatalf("external image is %d×%d pixels=%v external=%v", img.W, img.H, img.Pixels() != nil, img.External != nil)
	}
	img.Resize(9, 7)
	if img.W != 9 || img.H != 7 || img.Pixels() != nil {
		t.Fatalf("resized external image is %d×%d with pixels %v", img.W, img.H, img.Pixels() != nil)
	}
}
