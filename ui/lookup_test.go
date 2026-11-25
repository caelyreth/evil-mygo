package ui

import "testing"

// A look up comes to the innermost element under the pointer that takes
// it, where it is over it, and what that element looks up the window
// shows; over one that leaves it, it goes to the one around it.
func TestLookUpComesToTheElementUnderThePointer(t *testing.T) {
	var inner, outer []Point
	tt := NewTester(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool {
			if ev.Kind != InputLookUp {
				return false
			}
			outer = append(outer, Point{ev.X, ev.Y})
			return true
		}).Children(func() {
			word := Box(c).Absolute().Left(20).Top(30).Size(100, 20)
			word.HandleInput(func(ev InputEvent) bool {
				if ev.Kind != InputLookUp {
					return false
				}
				inner = append(inner, Point{ev.X, ev.Y})
				return c.Services().ShowDefinition("tide", Point{X: 20, Y: 46}, Font{Family: "Georgia", Size: 16})
			})
			Box(c).Absolute().Left(20).Top(80).Size(100, 20).HandleInput(func(InputEvent) bool { return false })
		})
	}, 200, 200)
	defer tt.rt.close()
	tt.Frame()

	tt.LookUp(30, 40)
	if len(inner) != 1 || inner[0] != (Point{10, 10}) || len(outer) != 0 {
		t.Fatalf("over the word: inner %v, outer %v; want one at [10 10] and none", inner, outer)
	}
	if got := tt.Definitions(); len(got) != 1 || got[0].Text != "tide" || got[0].At != (Point{20, 46}) || got[0].Font.Size != 16 {
		t.Fatalf("the window showed %+v", got)
	}
	tt.LookUp(30, 90)
	if len(outer) != 1 || outer[0] != (Point{30, 90}) {
		t.Fatalf("over an element leaving it: outer %v, want one at [30 90]", outer)
	}
}
