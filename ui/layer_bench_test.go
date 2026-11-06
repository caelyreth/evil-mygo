package ui

import "testing"

func BenchmarkUILayerRender(b *testing.B) {
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(16).Gap(8).Children(func() {
			Text(c, "Native layer")
			TextInput(c.Key("name"), new(string)).Label("Name")
			Button(c, "Apply")
		})
	}, 900, 500)
	defer layer.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		layer.Invalidate()
		layer.Render()
	}
}

func BenchmarkUILayerSurfaceInput(b *testing.B) {
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(16).Children(func() {
			Button(c.Key("input"), "Input").Width(200).Height(40)
		})
	}, 300, 200)
	defer layer.Close()
	surface := NewPlanarSurface(Rect{W: 300, H: 200})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		layer.HandleSurfaceInput(surface, InputEvent{
			Kind: InputPointerDown, X: 20, Y: 20, Button: 0,
		})
		layer.HandleSurfaceInput(surface, InputEvent{
			Kind: InputPointerUp, X: 20, Y: 20, Button: 0,
		})
	}
}
