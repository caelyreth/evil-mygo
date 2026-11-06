package ui

// GPUTexture is a bitmap-like GPU-composed image usable by a native
// renderer. WebGPU canvases, imported WebGPU textures, and live native UI
// layers implement it. Constructors remain backend-specific; this
// interface is the backend-neutral composition contract.
type GPUTexture interface {
	Size() (w, h int)
	Bitmap() *Bitmap
	Invalidate()
}
