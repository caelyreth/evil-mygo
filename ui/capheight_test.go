package ui

import "testing"

// A font's cap height stands under its ascent, near seven tenths of its
// em in a Latin face.
func TestFontMetricsHoldACapHeight(t *testing.T) {
	m := Font{Size: 20}.Metrics()
	if m.CapHeight < 0.6*20 || m.CapHeight > 0.8*20 || m.CapHeight >= m.Ascent {
		t.Fatalf("cap height %v of a 20 DIP font, ascent %v", m.CapHeight, m.Ascent)
	}
}
