package webgpu

import "testing"

func TestIdleImageSweepCountsFramesSinceLastSweep(t *testing.T) {
	stale := &textureEntry{lastFrame: 1}
	recent := &textureEntry{lastFrame: 200}
	r := &Renderer{images: map[uintptr]*textureEntry{1: stale, 2: recent}}
	r.frame = 240
	r.sweepImagesIfDue()
	if len(r.images) != 2 {
		t.Fatal("evicted an image within its 240-frame retention period")
	}
	r.frame = 241
	r.sweepImagesIfDue()
	if r.swept != 240 {
		t.Fatal("swept before another 120 frames elapsed")
	}
	// The first sweep due since frame 240 need not land on a multiple of 120.
	r.frame = 361
	r.sweepImagesIfDue()
	if r.swept != 361 || r.images[1] != nil || r.images[2] != recent {
		t.Fatalf("sweep at %d retained the wrong images: %v", r.swept, r.images)
	}
	// Reusing an image updates its timestamp, keeping it through a later sweep.
	recent.lastFrame = 480
	r.frame = 481
	r.sweepImagesIfDue()
	if r.images[2] != recent {
		t.Fatal("evicted a recently used image")
	}
}
