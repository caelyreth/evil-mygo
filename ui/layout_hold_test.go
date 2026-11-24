package ui

import (
	"fmt"
	"strings"
	"testing"
)

// A commit pass takes what a measuring pass kept of the items and lines,
// instead of building them again. What it lays out must be the same, so
// the trees are laid out with what was kept and without it, and every box
// compared.
func TestHoldKeepsLayout(t *testing.T) {
	for _, build := range holdTrees {
		layout := func() [][4]float32 {
			_, root := build()
			forgetLayout(root)
			layoutBox(root, root.w, root.h)
			return boxesOf(root)
		}
		noHold = true
		without := layout()
		noHold = false
		with := layout()
		noHold = false
		if len(with) != len(without) {
			t.Fatalf("%d boxes with what was kept, %d without", len(with), len(without))
		}
		for i := range with {
			if with[i] != without[i] {
				t.Errorf("box %d is %v, and %v without what was kept",
					i, boxString(with[i]), boxString(without[i]))
				break
			}
		}
	}
}

// What the commit passes take is what the measuring passes kept: a tree
// whose rows are measured and then laid out has both.
func TestHoldKeepsWhatMeasured(t *testing.T) {
	for _, build := range holdTrees {
		tt, root := build()
		kept, taken := tt.rt.flex.holdSaved, tt.rt.flex.holdUsed
		refreshed := tt.rt.flex.holdRefreshed
		forgetLayout(root)
		layoutBox(root, root.w, root.h)
		kept, taken = tt.rt.flex.holdSaved-kept, tt.rt.flex.holdUsed-taken
		refreshed = tt.rt.flex.holdRefreshed - refreshed
		t.Logf("kept %8d taken %8d refreshed %8d", kept, taken, refreshed)
		if taken == 0 {
			t.Errorf("no commit pass took what a measuring pass kept")
		}
		if taken > kept {
			t.Errorf("took %d of %d kept", taken, kept)
		}
		if refreshed == 0 {
			t.Errorf("no commit pass crossed the rows it took again")
		}
	}
}

// holdTrees are trees whose rows are measured at an indefinite height and
// then laid out at a definite one, which is what a commit pass takes what
// a measuring pass kept of.
var holdTrees = []func() (*Tester, *node){
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				for range 3 {
					coreRow(c).Width(280).Children(func() {
						coreBox(c).Size(40, 20).Shrink(0)
						coreBox(c).Width(60).Shrink(0)
						coreBox(c).Size(30, 30).Shrink(0)
					})
				}
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A row in a row: the inner one is measured, and
					// laid out at the height it was measured at.
					coreRow(c).Width(200).Children(func() {
						coreBox(c).Size(40, 20).Shrink(0)
						coreBox(c).Width(60).Shrink(0)
					})
					coreBox(c).Size(30, 30).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				// A row that wraps, with a child in percent: what it
				// resolved is kept, and taken again at its height.
				coreRow(c).Width(280).Wrap().Children(func() {
					for range 8 {
						coreBox(c).Width(60).Height(20).Shrink(0)
					}
					coreBox(c).WidthPercent(50).Height(20).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A child that grows into what the row has: its
					// size is resolved in the row's lines.
					coreBox(c).Width(40).Grow(1).Shrink(0)
					coreBox(c).Width(40).Grow(2).Shrink(0)
					coreBox(c).Height(24).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
	func() (*Tester, *node) {
		var root *node
		tt := coreNewTester(func(c *context) {
			root = coreColumn(c).Size(300, 400).Children(func() {
				coreRow(c).Width(280).Children(func() {
					// A child whose height is absolute, and bounded by a
					// percentage of the row's height: the bound applies
					// at the height the row is committed at, and not at the
					// indefinite height it was measured at, so the size
					// it is given at another height is not the one it was
					// measured at.
					coreBox(c).Width(60).Height(100).Shrink(0)
					coreBox(c).Width(60).Height(10).MinHeightPercent(50).Shrink(0)
				})
			})
		}, 300, 400)
		return tt, root
	},
}

// boxesOf returns the box of every element under root, in the order the
// layout laid them out.
func boxesOf(e *node) [][4]float32 {
	var out [][4]float32
	var walk func(*node)
	walk = func(e *node) {
		for c := e; c != nil; c = c.next {
			out = append(out, [4]float32{c.x, c.y, c.w, c.h})
			walk(c.first)
		}
	}
	walk(e)
	return out
}

func boxString(b [4]float32) string {
	s := make([]string, len(b))
	for i, v := range b {
		s[i] = fmt.Sprintf("%v", v)
	}
	return "{" + strings.Join(s, " ") + "}"
}

func TestHoldNonScrollingColumn(t *testing.T) {
	// Include plain, shrinking, percent-height, and growing non-scrolling columns
	// inside rows/boxes so measure-then-commit exercises both hold hits and
	// fallback invalidation, matching noHold pixel-for-pixel.
	buildCases := []struct {
		name     string
		wantHold bool
		build    func() (*Tester, *node)
	}{
		{
			name:     "plain-column-in-row",
			wantHold: true,
			build: func() (*Tester, *node) {
				var root *node
				tt := coreNewTester(func(c *context) {
					root = coreRow(c).Size(300, 200).Children(func() {
						coreColumn(c).Width(140).Children(func() {
							coreBox(c).Size(100, 25).Shrink(0)
							coreBox(c).Size(120, 35).Shrink(0)
						})
						coreBox(c).Size(100, 90).Shrink(0)
					})
				}, 300, 200)
				return tt, root
			},
		},
		{
			name:     "shrinking-column-falls-back",
			wantHold: false,
			build: func() (*Tester, *node) {
				var root *node
				tt := coreNewTester(func(c *context) {
					root = coreColumn(c).Size(140, 50).Children(func() {
						coreColumn(c).Width(120).MinHeight(0).Children(func() {
							coreBox(c).Size(100, 40)
							coreBox(c).Size(100, 40)
						})
					})
				}, 140, 50)
				return tt, root
			},
		},
		{
			name:     "percent-and-grow-children",
			wantHold: false,
			build: func() (*Tester, *node) {
				var root *node
				tt := coreNewTester(func(c *context) {
					root = coreRow(c).Size(300, 120).Children(func() {
						coreColumn(c).Size(140, 100).Children(func() {
							coreBox(c).Width(100).HeightPercent(50).Shrink(0)
							coreBox(c).Width(100).Height(20).Grow(1)
						})
					})
				}, 300, 120)
				return tt, root
			},
		},
	}
	for _, tc := range buildCases {
		t.Run(tc.name, func(t *testing.T) {
			noHold = true
			_, rootNo := tc.build()
			forgetLayout(rootNo)
			layoutBox(rootNo, rootNo.w, rootNo.h)
			without := boxesOf(rootNo)

			noHold = false
			tt, rootWith := tc.build()
			before := tt.rt.flex.holdUsed
			forgetLayout(rootWith)
			layoutBox(rootWith, rootWith.w, rootWith.h)
			used := tt.rt.flex.holdUsed - before
			with := boxesOf(rootWith)

			if len(with) != len(without) {
				t.Fatalf("box count %d vs noHold %d", len(with), len(without))
			}
			for i := range with {
				if with[i] != without[i] {
					t.Fatalf("box %d = %v, want %v", i, boxString(with[i]), boxString(without[i]))
				}
			}
			if tc.wantHold && used == 0 {
				t.Fatalf("expected non-scrolling Column to reuse flex hold, got holdUsed=0")
			}
			if !tc.wantHold && used != 0 {
				t.Fatalf("expected constrained Column not to reuse non-matching hold, got holdUsed=%d", used)
			}
		})
	}
}

func TestMeasureCacheEviction(t *testing.T) {
	var box *node
	_ = coreNewTester(func(c *context) {
		box = coreColumn(c).Children(func() {
			coreText(c, "word one two three four five six seven eight nine ten eleven twelve")
		})
	}, 300, 200)
	if box == nil {
		t.Fatal("missing test box")
	}
	widths := []float32{60, 100, 140, 180, 220, 260}
	for _, w := range widths {
		_ = naturalHeight(box, w, inf)
	}
	if int(box.nmeasure) != len(box.measures) {
		t.Fatalf("nmeasure = %d, want %d", box.nmeasure, len(box.measures))
	}
	// The last 4 widths must reside in FIFO slots 0..3 with newest at index 3.
	wantWidths := widths[2:]
	for i, w := range wantWidths {
		if box.measures[i].availW != w {
			t.Fatalf("measures[%d].availW = %v, want %v", i, box.measures[i].availW, w)
		}
		if got, want := naturalHeight(box, w, inf), box.measures[i].h; got != want {
			t.Fatalf("width %v returned %v, want cached %v", w, got, want)
		}
	}
}
