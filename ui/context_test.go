package ui

import "testing"

func TestContextPixelsPerDIP(t *testing.T) {
	var got float32
	tt := NewTester(func(c *Context) {
		got = c.PixelsPerDIP()
	}, 100, 80)
	defer tt.rt.close()
	tt.SetScale(2)
	tt.Frame()
	if got != 2 {
		t.Fatalf("PixelsPerDIP = %g, want 2", got)
	}
}
