//go:build darwin && !ios

package darwin

import (
	"strconv"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/internal/platform"
)

// A Palette submenu shows in place of its item as a row of swatches MyGo
// draws, as Finder's tag colors: compact, its discs at the size of the
// menu's icons and as close, standing where the menu's icons and labels
// start. The system's palette menu (macOS 14) draws its swatches at one
// size whatever they are given and spreads them over the menu's width.
//
// The row is one view; each swatch is a disc of its color, a ring while
// the pointer or the keys are on it, and a check while its item is
// checked, with a tooltip and an accessibility element of its label.
// Choosing one sends the menu's action from an item standing for it, as
// choosing any item does, and closes the menu.
const (
	swatchSize   = platform.MenuIconSize
	swatchGap    = 9
	swatchLead   = 16
	swatchHeight = 26
	// The ring stands this far out of the disc, this wide.
	swatchRingOut   = 2.5
	swatchRingWidth = 1.5
)

// swatchRow is what a row's view shows and chooses.
type swatchRow struct {
	items  []*platform.MenuItem
	stands []id // the items standing for the swatches, which send the action
	rings  []id
	checks []id
	// tips and elements are the tooltips' owners and the accessibility
	// elements, which the row holds.
	tips, elements []id
	hovered        int
}

// swatchRows are the rows of the menus shown, by view.
var swatchRows = map[id]*swatchRow{}

func registerSwatchClasses() {
	if hasClass("MyGoMenuSwatchRow") {
		return
	}
	classDef("MyGoMenuSwatchRow", "NSView", nil, []objc.MethodDef{
		method("mouseEntered:", func(self id, _ objc.SEL, ev id) { hoverSwatch(self, swatchAt(self, ev)) }),
		method("mouseMoved:", func(self id, _ objc.SEL, ev id) { hoverSwatch(self, swatchAt(self, ev)) }),
		method("mouseExited:", func(self id, _ objc.SEL, _ id) { hoverSwatch(self, -1) }),
		method("mouseUp:", func(self id, _ objc.SEL, ev id) { chooseSwatch(self, swatchAt(self, ev)) }),
		method("acceptsFirstResponder", func(self id, _ objc.SEL) bool { return true }),
		method("canBecomeKeyView", func(self id, _ objc.SEL) bool { return true }),
		method("keyDown:", func(self id, _ objc.SEL, ev id) { swatchKeyDown(self, ev) }),
		method("isAccessibilityElement", func(self id, _ objc.SEL) bool { return true }),
		method("accessibilityRole", func(self id, _ objc.SEL) id { return nsString("AXGroup") }),
		method("accessibilityLabel", func(self id, _ objc.SEL) id {
			if item := send(self, "enclosingMenuItem"); item != 0 {
				return send(item, "title")
			}
			return 0
		}),
		method("accessibilityChildren", func(self id, _ objc.SEL) id {
			if r := swatchRows[self]; r != nil {
				return nsArray(r.elements...)
			}
			return 0
		}),
	})
	classDef("MyGoMenuSwatchElement", "NSAccessibilityElement", nil, []objc.MethodDef{
		method("accessibilityPerformPress", func(self id, _ objc.SEL) bool {
			i, _ := strconv.Atoi(goString(send(self, "accessibilityIdentifier")))
			return chooseSwatch(send(self, "accessibilityParent"), i)
		}),
	})
}

// buildSwatchRow makes the item of a Palette submenu's item it: a row of
// its submenu's swatches, which send action to the menu's target.
func (b *Backend) buildSwatchRow(it *platform.MenuItem, owner int, action objc.SEL) id {
	registerSwatchClasses()
	r := &swatchRow{hovered: -1}
	for _, sw := range it.Submenu.Items {
		if sw.Type != platform.MenuItemSeparator && sw.Visible {
			r.items = append(r.items, sw)
		}
	}
	n := float64(len(r.items))
	width := 2*swatchLead + n*swatchSize + max(0, n-1)*swatchGap
	item := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
		uintptr(nsString(it.Label)), 0, uintptr(nsString(""))))
	send(item, "setEnabled:", boolArg(it.Enabled))
	view := autorelease(msgInitRect(send(class("MyGoMenuSwatchRow"), "alloc"), sel("initWithFrame:"),
		NSRect{Size: NSSize{width, swatchHeight}}))
	send(view, "setAutoresizingMask:", 2) // NSViewWidthSizable: the swatches keep to the leading edge
	send(view, "setWantsLayer:", 1)
	layer := send(view, "layer")
	rim := send(msgFloatID(send(class("NSColor"), "blackColor"), sel("colorWithAlphaComponent:"), 0.12), "CGColor")
	ring := send(msgFloatID(send(class("NSColor"), "grayColor"), sel("colorWithAlphaComponent:"), 0.6), "CGColor")
	check := send(class("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", uintptr(nsString("checkmark")), 0)
	if check != 0 {
		config := msgFloat2ID(class("NSImageSymbolConfiguration"), sel("configurationWithPointSize:weight:"), swatchSize*0.55, 0.4) // NSFontWeightBold
		check = send(check, "imageWithSymbolConfiguration:", uintptr(config))
	}
	for i, sw := range r.items {
		c := platform.Color{R: 142, G: 142, B: 147, A: 255}
		if sw.Swatch != nil {
			c = *sw.Swatch
		}
		disc := NSRect{Origin: NSPoint{X: swatchLead + float64(i)*(swatchSize+swatchGap), Y: (swatchHeight - swatchSize) / 2},
			Size: NSSize{swatchSize, swatchSize}}
		ringLayer := autorelease(send(send(class("CALayer"), "alloc"), "init"))
		msgSetRect(ringLayer, sel("setFrame:"), NSRect{
			Origin: NSPoint{disc.Origin.X - swatchRingOut, disc.Origin.Y - swatchRingOut},
			Size:   NSSize{swatchSize + 2*swatchRingOut, swatchSize + 2*swatchRingOut}})
		msgSetFloat(ringLayer, sel("setCornerRadius:"), swatchSize/2+swatchRingOut)
		msgSetFloat(ringLayer, sel("setBorderWidth:"), swatchRingWidth)
		send(ringLayer, "setBorderColor:", uintptr(ring))
		send(ringLayer, "setHidden:", 1)
		send(layer, "addSublayer:", uintptr(ringLayer))
		r.rings = append(r.rings, ringLayer)

		discLayer := autorelease(send(send(class("CALayer"), "alloc"), "init"))
		msgSetRect(discLayer, sel("setFrame:"), disc)
		msgSetFloat(discLayer, sel("setCornerRadius:"), swatchSize/2)
		send(discLayer, "setBackgroundColor:", uintptr(send(nsColor(c), "CGColor")))
		msgSetFloat(discLayer, sel("setBorderWidth:"), 0.5)
		send(discLayer, "setBorderColor:", uintptr(rim))
		if !sw.Enabled {
			msgSetFloat(discLayer, sel("setOpacity:"), 0.4)
		}
		send(layer, "addSublayer:", uintptr(discLayer))

		mark := autorelease(msgInitRect(send(class("NSImageView"), "alloc"), sel("initWithFrame:"), disc))
		send(mark, "setImage:", uintptr(check))
		send(mark, "setImageScaling:", 0) // NSImageScaleNone: the symbol at its own size, centred
		// White on a dark color, black on a light one.
		ink := "blackColor"
		if keepOpenColorDark(c) {
			ink = "whiteColor"
		}
		send(mark, "setContentTintColor:", uintptr(send(class("NSColor"), ink)))
		send(mark, "setHidden:", boolArg(!(sw.Checked && (sw.Type == platform.MenuItemCheckbox || sw.Type == platform.MenuItemRadio))))
		send(mark, "setAccessibilityElement:", 0)
		send(view, "addSubview:", uintptr(mark))
		r.checks = append(r.checks, mark)

		tip := retain(nsString(sw.Label))
		msgRectIDUint(view, sel("addToolTipRect:owner:userData:"), disc, tip, 0)
		r.tips = append(r.tips, tip)

		stand := autorelease(send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:",
			uintptr(nsString(sw.Label)), uintptr(action), uintptr(nsString(""))))
		send(stand, "setTarget:", uintptr(b.menuTarget))
		send(stand, "setTag:", uintptr(sw.ID))
		send(stand, "setEnabled:", boolArg(sw.Enabled))
		track(owner, sw.ID, stand)
		r.stands = append(r.stands, stand)

		el := send(send(class("MyGoMenuSwatchElement"), "alloc"), "init")
		role := "AXButton"
		if sw.Type == platform.MenuItemRadio || sw.Type == platform.MenuItemCheckbox {
			role = "AXRadioButton"
			if sw.Type == platform.MenuItemCheckbox {
				role = "AXCheckBox"
			}
			send(el, "setAccessibilityValue:", uintptr(send(class("NSNumber"), "numberWithBool:", boolArg(sw.Checked))))
		}
		send(el, "setAccessibilityRole:", uintptr(nsString(role)))
		send(el, "setAccessibilityLabel:", uintptr(nsString(sw.Label)))
		send(el, "setAccessibilityIdentifier:", uintptr(nsString(strconv.Itoa(i))))
		send(el, "setAccessibilityParent:", uintptr(view))
		send(el, "setAccessibilityEnabled:", boolArg(sw.Enabled))
		msgSetRect(el, sel("setAccessibilityFrameInParentSpace:"), disc)
		r.elements = append(r.elements, el)
	}
	const options = 0x01 | 0x02 | 0x80 | 0x200 // entered/exited, moved, active always, visible rect
	tracking := msgInitMenuTrack(send(class("NSTrackingArea"), "alloc"),
		sel("initWithRect:options:owner:userInfo:"), NSRect{}, options, view, 0)
	send(view, "addTrackingArea:", uintptr(tracking))
	release(tracking)
	send(item, "setView:", uintptr(view))
	swatchRows[view] = r
	track(owner, it.ID, item)
	return item
}

// dropSwatchRow lets go of what the row of view holds.
func dropSwatchRow(view id) {
	r := swatchRows[view]
	if r == nil {
		return
	}
	for _, x := range r.tips {
		release(x)
	}
	for _, x := range r.elements {
		release(x)
	}
	delete(swatchRows, view)
}

// swatchAt is the swatch of the row view under the event's pointer, -1
// for none: each holds the room halfway to the next.
func swatchAt(view, ev id) int {
	r := swatchRows[view]
	if r == nil {
		return -1
	}
	p := msgPointFromView(view, sel("convertPoint:fromView:"), msgPoint(ev, sel("locationInWindow")), 0)
	x := p.X - swatchLead + swatchGap/2
	if x < 0 {
		return -1
	}
	if i := int(x / (swatchSize + swatchGap)); i < len(r.items) {
		return i
	}
	return -1
}

// hoverSwatch rings the swatch i of the row view, none for -1.
func hoverSwatch(view id, i int) {
	r := swatchRows[view]
	if r == nil || r.hovered == i {
		return
	}
	r.hovered = i
	for j, ring := range r.rings {
		send(ring, "setHidden:", boolArg(j != i || !r.items[j].Enabled))
	}
}

// chooseSwatch chooses the swatch i of the row view and closes its menu,
// and the menus it opened from.
func chooseSwatch(view id, i int) bool {
	r := swatchRows[view]
	if r == nil || i < 0 || i >= len(r.stands) || !r.items[i].Enabled {
		return false
	}
	stand := send(r.stands[i], "retain")
	defer release(stand)
	menu := send(send(view, "enclosingMenuItem"), "menu")
	for menu != 0 {
		up := send(menu, "supermenu")
		if up == 0 {
			send(menu, "cancelTracking")
			break
		}
		menu = up
	}
	app := send(class("NSApplication"), "sharedApplication")
	send(app, "sendAction:to:from:", uintptr(send(stand, "action")), uintptr(send(stand, "target")), uintptr(stand))
	return true
}

// swatchKeyDown moves along the row from the keyboard, chooses the swatch
// ringed, and moves on through the menu, as NSMenu does for its items.
func swatchKeyDown(view, ev id) {
	r := swatchRows[view]
	item := send(view, "enclosingMenuItem")
	if r == nil || item == 0 {
		return
	}
	menu := send(item, "menu")
	switch sendInt(ev, "keyCode") {
	case 123: // Left
		hoverSwatch(view, max(0, r.hovered-1))
	case 124: // Right
		hoverSwatch(view, min(len(r.items)-1, r.hovered+1))
	case 36, 76, 49: // Return, keypad Enter, Space
		chooseSwatch(view, r.hovered)
	case 125: // Down
		highlightMenuItem(menu, 1)
	case 126: // Up
		highlightMenuItem(menu, -1)
	case 53: // Escape
		send(menu, "cancelTracking")
	}
}

// keySwatchRow rings the checked swatch of the row view, else its first,
// as the keys come onto it, and none as they leave.
func keySwatchRow(view id, on bool) {
	r := swatchRows[view]
	if r == nil {
		return
	}
	if !on {
		hoverSwatch(view, -1)
		return
	}
	if r.hovered >= 0 {
		return
	}
	at := 0
	for i, sw := range r.items {
		if sw.Checked {
			at = i
			break
		}
	}
	hoverSwatch(view, at)
}
