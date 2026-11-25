package ui

import "testing"

// Over the system's material, a drawn menu's panel keeps
// menuMaterialShows of its background, or as much as its style asks.
func TestMenuPanelOverMaterial(t *testing.T) {
	for _, c := range []struct {
		over float32
		want uint8
	}{{0, 140}, {0.85, 216}} { // menuMaterialShows of 255, and 0.85 of it
		var got uint8
		tt := coreNewTester(func(ctx *context) {
			panel := coreBox(ctx)
			b := &drawnRows{d: &drawnMenu{style: PopoverMenuOptions{OverMaterial: c.over}}, t: LightTheme(), material: true}
			b.style(panel, 0, false)
			got = panel.bg.A
		}, 100, 100)
		tt.Frame()
		tt.rt.close()
		if d := int(got) - int(c.want); d < -1 || d > 1 {
			t.Errorf("over %v: alpha %d, want %d", c.over, got, c.want)
		}
	}
}
