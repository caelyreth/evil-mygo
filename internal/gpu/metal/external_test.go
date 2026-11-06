//go:build darwin

package metal

import (
	"testing"

	"github.com/egoist/mygo/internal/scene"
)

func TestExternalTextureIsSampledWithoutUpload(t *testing.T) {
	r := &Renderer{device: 123}
	img := scene.NewExternalImage(8, 6, ptr(123), ptr(456), nil)
	if got := r.imageTexture(img); got != 456 {
		t.Fatalf("external texture resolved to %d, want 456", got)
	}
	other := scene.NewExternalImage(8, 6, ptr(999), ptr(456), nil)
	if got := r.imageTexture(other); got != 0 {
		t.Fatalf("texture from another device resolved to %d, want 0", got)
	}
}
