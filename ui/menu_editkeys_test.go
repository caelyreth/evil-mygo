package ui

import "testing"

// The editing items of a menu show the keys the text takes them by, as
// the Edit menu shows them.
func TestEditItemsShowTheirKeys(t *testing.T) {
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Selectable().ContextMenu(func(m *Menu) { m.EditItems() }).Children(func() {
			coreText(c, "Alpha")
		})
	}, 300, 200)
	selectBetween(t, tt, "Alpha", 0, "Alpha", 5)
	x, y := selectableAt(t, tt, "Alpha", 2)
	tt.RightClickAt(x, y)
	m, _ := tt.currentMenu()
	if m == nil {
		t.Fatal("no menu")
	}
	keys := map[string]string{}
	for _, it := range m.Items {
		keys[it.Label] = it.Accelerator
	}
	if keys["Copy"] != accelerator(Cmd, KeyC) || keys["Select All"] != accelerator(Cmd, KeyA) {
		t.Fatalf("keys %q", keys)
	}
}
