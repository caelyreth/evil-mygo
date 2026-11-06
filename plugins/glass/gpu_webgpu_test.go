//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package glass

import (
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
	"github.com/egoist/mygo/internal/gpu/webgpu"
)

func TestWebGPURendersGlass(t *testing.T) {
	pix, err := webgpu.RenderOffscreen(testScene())
	if err != nil {
		t.Fatal(err)
	}
	if len(pix) != 320*140*4 {
		t.Fatalf("rendered %d bytes, want %d", len(pix), 320*140*4)
	}
	gputest.Compare(t, "glass-webgpu", pix, 320*4, testScene())
}
