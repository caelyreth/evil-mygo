//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/internal/platform"
)

// paletteItem is a Palette item of three swatches, the second checked.
func paletteItem() *platform.MenuItem {
	swatch := func(id int, c platform.Color, on bool) *platform.MenuItem {
		return &platform.MenuItem{ID: id, Label: "Color " + string(rune('A'+id)), Type: platform.MenuItemRadio, Checked: on,
			Visible: true, Enabled: true, Swatch: &c, Icon: platform.SwatchIcon(c)}
	}
	return &platform.MenuItem{ID: 2, Label: "Color", Type: platform.MenuItemSubmenu, Visible: true, Enabled: true,
		Submenu: &platform.Menu{Palette: true, Items: []*platform.MenuItem{
			swatch(3, platform.Color{R: 220, A: 255}, false),
			swatch(4, platform.Color{G: 180, A: 255}, true),
			swatch(5, platform.Color{B: 200, A: 255}, false),
		}}}
}

// checkSwatchRow checks that item is a row of the swatches of paletteItem,
// each sent with action, the second checked, rung as the keys come on.
func checkSwatchRow(t *testing.T, item id, action string) {
	t.Helper()
	view := send(item, "view")
	r := swatchRows[view]
	if view == 0 || r == nil {
		t.Fatal("the Color item is not a row of swatches")
	}
	if send(item, "submenu") != 0 {
		t.Error("the row opens a submenu")
	}
	if got := len(r.stands); got != 3 {
		t.Fatalf("the row has %d swatches, want 3", got)
	}
	for i, want := range []int{3, 4, 5} {
		stand := r.stands[i]
		if got := sendInt(stand, "tag"); got != want {
			t.Errorf("swatch %d has tag %d, want %d", i, got, want)
		}
		if objc.SEL(send(stand, "action")) != sel(action) {
			t.Errorf("swatch %d is not sent with %s", i, action)
		}
		if shown := !sendBool(r.checks[i], "isHidden"); shown != (want == 4) {
			t.Errorf("swatch %d checked = %v, want %v", i, shown, want == 4)
		}
	}
	keySwatchRow(view, true)
	if r.hovered != 1 || sendBool(r.rings[1], "isHidden") {
		t.Errorf("the keys coming on ring swatch %d, want the checked one", r.hovered)
	}
	if w := msgRect(view, sel("frame")).Size.Width; w != 2*swatchLead+3*swatchSize+2*swatchGap {
		t.Errorf("the row is %v wide, want the swatches' room", w)
	}
}

// TestMenuSwatchRow builds a Palette item as a row of swatches MyGo draws,
// whose swatches tell the menu's target.
func TestMenuSwatchRow(t *testing.T) {
	load()
	registerMenuTarget()
	withPool(func() {
		b := &Backend{}
		m := &platform.Menu{Items: []*platform.MenuItem{{ID: 1, Label: "Rename", Visible: true, Enabled: true}, paletteItem()}}
		owner := newOwner()
		menu := b.buildMenu(m, "", owner)
		item := send(menu, "itemAtIndex:", 1)
		view := send(item, "view")
		checkSwatchRow(t, item, "mygoMenuItemClicked:")
		dropOwner(owner)
		if swatchRows[view] != nil {
			t.Error("the row is held once its menu is let go of")
		}
	})
}
