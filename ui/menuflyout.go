package ui

import (
	"math"
	"slices"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/surface"
)

// menuFlyouts is a menu drawn in flyouts of the window (FlyoutMenus), d's:
// one for the menu and one for each submenu open, each a view of its own
// building the panel of its level from what the window's build of the
// menu last collected. The levels share d.menu, the menu's state, as the
// panels of a menu drawn in the window do: the highlight of each, the
// submenus open and the level the focus goes to. What one changes there
// asks the window for a frame, whose build opens and closes the flyouts
// to match.
type menuFlyouts struct {
	rt *engine // the window's
	d  *drawnMenu
	// pm is the menu as the window's build last collected it, in run,
	// to be drawn in theme at zoom.
	pm     *platform.Menu
	run    *menuRun
	theme  *Theme
	zoom   float32
	rtl    bool
	levels []*flyoutLevel
	// chosen is the item clicked in a level, for the window's next build
	// to report; closed is set once the menu closes.
	chosen *platform.MenuItem
	closed bool
}

// flyoutLevel is the flyout of a level of the menu: sub is the row whose
// submenu it shows, 0 for the menu, and label that row's. rows are the
// boxes of its rows opening a submenu, and refs the item of each of its
// rows, as its last build left them.
type flyoutLevel struct {
	f     surface.Flyout
	sub   uint64
	label string
	rows  map[uint64]Rect
	refs  map[uint64]menuRef
}

// flownMenuBuilt shows the menu d drawn for e, which its build filled as
// m, in flyouts of the window (FlyoutMenus): it opens the menu's flyout,
// or, where the window opens none, draws it in the window from then on.
// It returns the item chosen from it, nil if none.
func (rt *engine) flownMenuBuilt(e *node, d *drawnMenu, m *Menu) *platform.MenuItem {
	run, pm := m.run, m.menu
	if it, done := d.settle(pm, run); done {
		rt.menu.drawn = nil // settleMenuFlyouts closes its flyouts
		return it
	}
	f := d.flyouts
	if f == nil {
		f = &menuFlyouts{rt: rt, d: d}
	}
	f.pm, f.run, f.theme, f.zoom = pm, run, e.theme(), rt.zoomFactor()
	if d.flyouts == nil {
		if !f.openMenu(e) {
			d.inWindow = true
			return rt.drawnMenuBuilt(e, d, m)
		}
		d.flyouts, rt.menu.flown = f, f
	}
	chosen := f.chosen
	f.chosen = nil
	if f.closed {
		rt.menu.drawn = nil
		return chosen
	}
	f.sync()
	if d.menu.below {
		e.expanded = true
	}
	refs := map[uint64]menuRef{}
	for _, lv := range f.levels {
		for id, ref := range lv.refs {
			refs[id] = ref
		}
	}
	d.keepHighlighted(rt, refs)
	return chosen
}

// settleMenuFlyouts closes the flyouts of a menu that no longer shows,
// once a pass of the view is done: closed, or gone with its element.
func (rt *engine) settleMenuFlyouts() {
	f := rt.menu.flown
	if f == nil {
		return
	}
	if d := rt.menu.drawn; d != nil && d.flyouts == f && !f.closed {
		return
	}
	rt.menu.flown = nil
	f.closed = true
	f.closeFrom(0)
}

// openMenu opens the flyout of the menu, as its element asked: below a
// menu button, as its options place it, or at the point a context menu
// opened, after it, before it right to left. It reports whether the
// window opened one.
func (f *menuFlyouts) openMenu(e *node) bool {
	s, z := &f.d.menu, f.zoom
	b := e.Bounds()
	rtl := e.rtl()
	f.rtl = rtl
	spec := surface.FlyoutSpec{Placement: "bottom-start"}
	if s.below {
		spec.Anchor = [4]float64{float64(b.X * z), float64(b.Y * z), float64(b.W * z), float64(b.H * z)}
		at, self := s.buttonAnchors()
		spec.Placement = flyoutPlacement(at, self, rtl)
		spec.Gap = int(math.Round(float64(f.theme.Space(1) * z)))
	} else {
		spec.Anchor = [4]float64{float64((b.X + s.dx) * z), float64((b.Y + s.dy) * z), 0, 0}
		if rtl {
			spec.Placement = "bottom-end"
		}
	}
	return f.open(&flyoutLevel{}, spec, true)
}

// material has spec show the system's material of menus behind the
// flyout, cut to the panel's corners.
func (f *menuFlyouts) material(spec *surface.FlyoutSpec) {
	spec.Material, spec.Radius = true, float64(f.d.radius(f.theme)*f.zoom)
}

// flyoutPlacement returns the flyout placement (mygo.Placement) of a
// menu attached to its button by the points at of the button's box and
// self of the menu's (PopoverMenuButtonOptions).
func flyoutPlacement(at, self Anchor, rtl bool) string {
	ax, ay := at.physical(rtl).fractions()
	sx, sy := self.physical(rtl).fractions()
	along := func(f float32) string {
		switch {
		case f == 0:
			return "-start"
		case f == 1:
			return "-end"
		}
		return ""
	}
	switch {
	case ay == 0 && sy == 1:
		return "top" + along(sx)
	case ax == 1 && sx == 0:
		return "right" + along(sy)
	case ax == 0 && sx == 1:
		return "left" + along(sy)
	}
	return "bottom" + along(sx)
}

// open opens the flyout of lv, placed by spec, taking the keyboard as it
// shows when focus is set, and reports whether the window opened it.
func (f *menuFlyouts) open(lv *flyoutLevel, spec surface.FlyoutSpec, focus bool) bool {
	f.material(&spec)
	fl := f.rt.host.flyout(spec, coreView(func(c *context) { f.build(c, lv) }))
	if fl == nil {
		return false
	}
	lv.f = fl
	f.levels = append(f.levels, lv)
	fl.OnClose(func() { f.dismissed(lv) })
	fl.Show(focus)
	return true
}

// sync opens and closes the flyouts of the submenus to match the menu's
// state, as the levels' builds changed it, and has every level build
// again from what the window's build collected.
func (f *menuFlyouts) sync() {
	s := &f.d.menu
	n := 1
	for n < len(f.levels) && n < len(s.levels) && f.levels[n].sub == s.levels[n].sub {
		n++
	}
	f.closeFrom(n)
	for k := len(f.levels); k < len(s.levels); k++ {
		above := f.levels[k-1]
		row, ok := above.rows[s.levels[k].sub]
		if !ok || !f.openSubmenu(k, above, row) {
			s.levels = s.levels[:k]
			break
		}
	}
	for _, lv := range f.levels {
		lv.f.Invalidate()
	}
}

// openSubmenu opens the flyout of the submenu at level k, which the row
// of above in box row opens: after the row, its first item lined up with
// it, before it right to left. It takes the keyboard when the keys opened
// it.
func (f *menuFlyouts) openSubmenu(k int, above *flyoutLevel, row Rect) bool {
	s, z := &f.d.menu, f.zoom
	sub := s.levels[k].sub
	// Up by the panel's padding and border, which its first item is in.
	pad := f.theme.Space(1) + 1
	spec := surface.FlyoutSpec{
		Parent:    above.f,
		Anchor:    [4]float64{float64(row.X * z), float64((row.Y - pad) * z), float64(row.W * z), float64((row.H + 2*pad) * z)},
		Placement: "right-start",
	}
	if f.rtl {
		spec.Placement = "left-start"
	}
	return f.open(&flyoutLevel{sub: sub, label: above.refs[sub].label}, spec, s.focus == k)
}

// closeFrom closes the flyouts of the levels from k on, as the menu's
// state no longer has them, the deepest first.
func (f *menuFlyouts) closeFrom(k int) {
	if k >= len(f.levels) {
		return
	}
	gone := f.levels[k:]
	f.levels = f.levels[:k:k]
	for i := len(gone) - 1; i >= 0; i-- {
		gone[i].f.Close()
	}
}

// dismissed takes the flyout of lv closing, unless the menu closed it: the
// user dismissed it, by a press outside it, by its Escape or by the
// keyboard going elsewhere, or it closed with the one holding it. The
// menu closes with its own flyout, and a submenu closes, the focus going
// to the menu holding it.
func (f *menuFlyouts) dismissed(lv *flyoutLevel) {
	k := slices.Index(f.levels, lv)
	if k < 0 {
		return
	}
	f.levels = f.levels[:k]
	s := &f.d.menu
	switch {
	case k == 0:
		f.closed = true
	case k < len(s.levels):
		s.levels = s.levels[:k]
		s.focus = k - 1
	}
	f.rt.requestFrame()
}

// build builds the panel of the level lv in its flyout's view, from what
// the window's build of the menu last collected, in its theme: the menu's
// items, or those of the submenu the row lv.sub opens. It asks the window
// for a frame when it changed the menu's state.
func (f *menuFlyouts) build(c *context, lv *flyoutLevel) {
	k := slices.Index(f.levels, lv)
	s := &f.d.menu
	if f.closed || k < 0 || k >= len(s.levels) {
		return
	}
	c.SetTheme(f.theme)
	c.SetZoom(f.zoom)
	// The panel draws all the flyout shows: its corners, rounded, show
	// what is behind them.
	c.Root().Background(Transparent)
	menu := f.pm
	if k > 0 {
		ref, ok := f.levels[k-1].refs[lv.sub]
		it := menuItemByID(f.pm.Items, ref.seq)
		if !ok || it == nil || it.Submenu == nil {
			return
		}
		menu = it.Submenu
	}
	width := f.run.widths[menu]
	rt := c.rt
	levels, focus := slices.Clone(s.levels), s.focus
	open := true
	parts := &menuParts{Trigger: c.root, c: c, open: &open, s: s, level: -1, built: make([]menuBuilt, k, k+1)}
	parts.moved = [2]float32{rt.pointerX, rt.pointerY} != s.pointer
	b := &drawnRows{d: f.d, run: f.run, parts: parts, t: c.theme, flyout: true, material: rt.host.vibrancy(), refs: map[uint64]menuRef{}, rows: map[uint64]Rect{}}
	panel := parts.panel(k, nil, func(panel *node) {
		b.style(panel, width, false)
		if lv.label != "" {
			panel.Label(lv.label)
		}
		b.items(menu, width)
	})
	if s.focus == k {
		panel.Focus()
		s.focus = -1
		if !rt.host.active() {
			rt.host.post(lv.f.Focus)
		}
	}
	s.last = [2]float32{rt.pointerX, rt.pointerY}
	lv.rows, lv.refs = b.rows, b.refs
	if b.chosen != nil {
		f.chosen = b.chosen
	}
	if parts.closed {
		f.closed = true
	}
	if b.chosen != nil || parts.closed || focus != s.focus || !slices.Equal(levels, s.levels) {
		f.rt.requestFrame()
	}
}
