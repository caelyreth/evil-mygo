package ui

import (
	"fmt"
	"strings"
	"testing"
)

// Compare edit allocation for two document sizes; allocation should follow
// chunk size and tree depth rather than document byte length.
func BenchmarkTextBufferReplace(b *testing.B) {
	for _, size := range []int{1 << 20, 8 << 20} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			doc := NewTextBuffer(strings.Repeat("a", size))
			b.ReportAllocs()
			for b.Loop() {
				doc.Replace(size/2, size/2+1, "b")
			}
		})
	}
}

func BenchmarkTextAreaBufferType(b *testing.B) {
	for _, lines := range []int{10000, 100000} {
		b.Run(fmt.Sprint(lines), func(b *testing.B) {
			doc := NewTextBuffer(codeLines(lines))
			tt := bufferTester(doc, true)
			ed := tt.rt.states[tt.rt.focused].editor
			ed.move(10, false)
			tt.Frame()
			b.ReportAllocs()
			for b.Loop() {
				tt.Type("a")
			}
		})
	}
}

func BenchmarkTextAreaLinesType(b *testing.B) {
	doc := NewTextBuffer(codeLines(1000))
	tt := coreNewTester(func(c *context) {
		coreColumn(c).Children(func() {
			coreTextAreaBuffer(c, doc).Lines(3, 25).FillWidth().AutoFocus()
		})
	}, 800, 600)
	ed := tt.rt.states[tt.rt.focused].editor
	ed.move(10, false)
	tt.Frame()
	b.ReportAllocs()
	for b.Loop() {
		tt.Type("a")
	}
}

func BenchmarkBoundsForRange(b *testing.B) {
	s := strings.Repeat("abcde\u0301 ", 150) // 1050 runes across ASCII + combining diacritics
	tt := coreNewTester(func(c *context) {
		coreTextArea(c, &s).Fill().AutoFocus()
	}, 800, 600)
	tt.Press(20, 20)
	tt.Release(20, 20)
	tt.Frame()
	client := tt.rt.states[tt.rt.focused].editor.client
	r := TextInputRange{Start: 500, End: 501}
	if _, _, ok := client.BoundsForRange(r); !ok {
		b.Fatal("BoundsForRange returned ok=false")
	}
	b.ReportAllocs()
	for b.Loop() {
		client.BoundsForRange(r)
	}
}

func TestTextBufferEditAllocationIsBounded(t *testing.T) {
	doc := NewTextBuffer(strings.Repeat("a", 8<<20))
	allocated := testing.AllocsPerRun(20, func() { doc.Replace(100, 101, "b") })
	if allocated > 100 {
		t.Fatalf("small indexed edit allocates %.0f objects", allocated)
	}
	if doc.ByteLen() != 8<<20 || doc.Slice(99, 102) != "aba" {
		t.Fatal("small edit corrupted document")
	}
}
