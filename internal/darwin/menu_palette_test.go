//go:build darwin && !ios

package darwin

import (
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/internal/platform"
)

// TestMenuPalette builds a Palette submenu as the system's palette, from
// macOS 14: a swatch for each item, tagged and targeted as the menu's
// other items, the checked radio's checked, choosing one at a time, and
// without the icon that stands for it in a submenu.
func TestMenuPalette(t *testing.T) {
	load()
	registerMenuTarget()
	LinkMenuIcons()
	defer func() { menuImage = nil }()
	if !respondsTo(class("NSMenu"), "paletteMenuWithColors:titles:selectionHandler:") {
		t.Skip("no palette menus before macOS 14")
	}
	withPool(func() {
		b := &Backend{}
		registerMenuTarget()
		swatch := func(id int, c platform.Color, on bool) *platform.MenuItem {
			return &platform.MenuItem{ID: id, Label: "Color", Type: platform.MenuItemRadio, Checked: on,
				Visible: true, Enabled: true, Swatch: &c, Icon: platform.SwatchIcon(c)}
		}
		colors := []platform.Color{{R: 220, A: 255}, {G: 180, A: 255}, {B: 200, A: 255}}
		m := &platform.Menu{Items: []*platform.MenuItem{
			{ID: 1, Label: "Rename", Visible: true, Enabled: true},
			{ID: 2, Label: "Color", Type: platform.MenuItemSubmenu, Visible: true, Enabled: true, Submenu: &platform.Menu{Palette: true, Items: []*platform.MenuItem{
				swatch(3, colors[0], false),
				swatch(4, colors[1], true),
				swatch(5, colors[2], false),
			}}},
		}}
		owner := newOwner()
		menu := b.buildMenu(m, "", owner)
		defer dropOwner(owner)
		pal := send(send(menu, "itemAtIndex:", 1), "submenu")
		if pal == 0 || sendInt(pal, "presentationStyle") != 1 {
			t.Fatal("the Color item's submenu is not a palette")
		}
		if got := sendInt(pal, "selectionMode"); got != 1 {
			t.Errorf("selectionMode = %d, want select one", got)
		}
		if got := sendInt(pal, "numberOfItems"); got != 3 {
			t.Fatalf("the palette has %d swatches, want 3", got)
		}
		for i, want := range []int{3, 4, 5} {
			item := send(pal, "itemAtIndex:", uintptr(i))
			if got := sendInt(item, "tag"); got != want {
				t.Errorf("swatch %d has tag %d, want %d", i, got, want)
			}
			if send(item, "target") != b.menuTarget || objc.SEL(send(item, "action")) != sel("mygoMenuItemClicked:") {
				t.Errorf("swatch %d does not tell the menu's target", i)
			}
			if on := sendInt(item, "state") == 1; on != (want == 4) {
				t.Errorf("swatch %d checked = %v, want %v", i, on, want == 4)
			}
			if send(item, "image") == menuImage(platform.SwatchIcon(colors[i])) {
				t.Errorf("swatch %d shows the submenu's icon in place of the system's swatch", i)
			}
		}
	})
}

// TestContextMenuPalette builds a palette in a page's context menu too,
// its swatches telling the window.
func TestContextMenuPalette(t *testing.T) {
	load()
	registerMenuTarget()
	if !hasPalettes() {
		t.Skip("no palette menus before macOS 14")
	}
	if theBackend == nil {
		theBackend = &Backend{}
		defer func() { theBackend = nil }()
	}
	withPool(func() {
		red := platform.Color{R: 220, A: 255}
		c := &contextFill{was: map[int]*platform.MenuItem{}}
		item := c.app(&platform.MenuItem{ID: 1, Label: "Color", Type: platform.MenuItemSubmenu, Visible: true, Enabled: true,
			Submenu: &platform.Menu{Palette: true, Items: []*platform.MenuItem{
				{ID: 2, Label: "Red", Type: platform.MenuItemRadio, Visible: true, Enabled: true, Swatch: &red},
			}}})
		pal := send(item, "submenu")
		if pal == 0 || sendInt(pal, "presentationStyle") != 1 {
			t.Fatal("the Color item's submenu is not a palette")
		}
		if objc.SEL(send(send(pal, "itemAtIndex:", 0), "action")) != sel("mygoContextItemClicked:") {
			t.Error("the swatch does not tell the window")
		}
	})
}
