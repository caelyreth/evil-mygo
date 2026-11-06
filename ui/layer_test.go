package ui

import "testing"

func TestUILayerRendersAndRoutesInput(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		uiColumn := Column(c).Fill().Padding(10)
		uiColumn.Children(func() {
			button := ButtonBase(c.Key("layer-button")).Width(80).Height(30).Label("Layer button")
			button.Children(func() { Text(c, "Click") })
			if button.Clicked() {
				clicks++
			}
		})
	}, 100, 60)
	defer layer.Close()

	if w, h := layer.Bitmap().Size(); w != 100 || h != 60 {
		t.Fatalf("layer bitmap = %d×%d, want 100×60", w, h)
	}
	if layer.img.Scene == nil || len(layer.img.Scene.Ops) == 0 {
		t.Fatal("layer rendered no scene")
	}

	layer.HandleInput(InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleInput(InputEvent{Kind: InputPointerUp, X: 20, Y: 20, Button: 0})
	layer.Render()
	if clicks != 1 {
		t.Fatalf("layer click count = %d, want 1", clicks)
	}
}

func TestUILayerResizeAndClose(t *testing.T) {
	layer := NewUILayer(func(c *Context) { Text(c, "hello") }, 80, 40)
	layer.Resize(160, 90)
	if w, h := layer.Size(); w != 160 || h != 90 {
		t.Fatalf("resized layer = %d×%d", w, h)
	}
	layer.Close()
	if !layer.closed || layer.img.Scene != nil {
		t.Fatal("closed layer kept its scene")
	}
}

func TestUILayerScaleKeepsInputInDIPs(t *testing.T) {
	clicks := 0
	layer := NewUILayerWithScale(func(c *Context) {
		Column(c).Fill().Padding(10).Children(func() {
			button := ButtonBase(c.Key("scaled-layer")).Width(80).Height(30).Label("Scaled layer")
			button.Children(func() { Text(c, "Click") })
			if button.Clicked() {
				clicks++
			}
		})
	}, 100, 60, 2)
	defer layer.Close()

	if w, h := layer.Size(); w != 100 || h != 60 {
		t.Fatalf("layer size = %d×%d, want 100×60 DIPs", w, h)
	}
	if scale := layer.Scale(); scale != 2 {
		t.Fatalf("layer scale = %g, want 2", scale)
	}
	if w, h := layer.Bitmap().Size(); w != 200 || h != 120 {
		t.Fatalf("layer bitmap = %d×%d, want 200×120 pixels", w, h)
	}

	surface := NewPlanarSurface(Rect{W: 100, H: 60})
	layer.HandleSurfaceInput(surface, InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleSurfaceInput(surface, InputEvent{Kind: InputPointerUp, X: 20, Y: 20, Button: 0})
	layer.Render()
	if clicks != 1 {
		t.Fatalf("scaled layer click count = %d, want 1", clicks)
	}
}

func TestUILayerResizePreservesScale(t *testing.T) {
	layer := NewUILayerWithScale(func(c *Context) { Text(c, "hello") }, 40, 30, 2)
	defer layer.Close()
	layer.Resize(75, 35)
	if w, h := layer.Size(); w != 75 || h != 35 {
		t.Fatalf("resized layer = %d×%d DIPs, want 75×35", w, h)
	}
	if w, h := layer.Bitmap().Size(); w != 150 || h != 70 {
		t.Fatalf("resized bitmap = %d×%d pixels, want 150×70", w, h)
	}
}

func TestUILayerAccessibility(t *testing.T) {
	name := "Ada"
	layer := NewUILayer(func(c *Context) {
		TextInput(c.Key("name"), &name).Label("Name")
	}, 180, 60)
	defer layer.Close()
	if a, ok := layer.Access("Name"); !ok || a.Role != RoleTextField {
		t.Fatalf("access = %+v, %v", a, ok)
	}
}

func TestUILayerSurfaceInput(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(10).Children(func() {
			button := ButtonBase(c.Key("surface-button")).Width(80).Height(30).Label("Surface button")
			button.Children(func() { Text(c, "Click") })
			if button.Clicked() {
				clicks++
			}
		})
	}, 100, 60)
	defer layer.Close()
	m := NewPlanarSurface(Rect{W: 100, H: 60})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerMove, X: 200, Y: 200})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerUp, X: 200, Y: 200, Button: 0})
	layer.Render()
	if clicks != 1 {
		t.Fatalf("surface click count = %d, want 1", clicks)
	}
}

func TestUILayerSurfaceInputCanceledPressClicksNothing(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(10).Children(func() {
			button := ButtonBase(c.Key("canceled-surface-button")).Width(80).Height(30).Label("Canceled")
			button.Children(func() { Text(c, "Click") })
			if button.Clicked() {
				clicks++
			}
		})
	}, 100, 60)
	defer layer.Close()
	m := NewPlanarSurface(Rect{W: 100, H: 60})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerUp, X: 200, Y: 200, Button: 0, Canceled: true})
	if layer.surfacePressed {
		t.Fatal("canceled surface press stayed captured")
	}
	layer.Render()
	if clicks != 0 {
		t.Fatalf("canceled surface press clicked %d times", clicks)
	}
}

func TestUILayerTextInputThroughSurface(t *testing.T) {
	name := ""
	layer := NewUILayer(func(c *Context) {
		Column(c).Fill().Padding(10).Children(func() {
			TextInput(c.Key("name"), &name).Width(140).Height(30).Label("Name")
		})
	}, 180, 60)
	defer layer.Close()
	m := NewPlanarSurface(Rect{W: 180, H: 60})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerUp, X: 20, Y: 20, Button: 0})
	layer.HandleInput(InputEvent{Kind: InputText, Text: "Ada"})
	layer.Render()
	if name != "Ada" {
		t.Fatalf("text field = %q, want Ada", name)
	}
	if active, _ := layer.TextInputActive(); !active {
		t.Fatal("text input is not active after the surface click")
	}
}

func TestUILayerCanFeedMetalCanvasSource(t *testing.T) {
	layer := NewUILayer(func(c *Context) {
		Text(c, "Layer source")
	}, 120, 40)
	defer layer.Close()
	canvas := NewMetalCanvas(120, 40, nil)
	canvas.SetSource(layer.Bitmap())
	if got := canvas.img.GPU.Source.Value(); got != layer.img {
		t.Fatalf("Metal source = %p, want layer image %p", got, layer.img)
	}
}

func TestUILayerDoesNotReplayClicks(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		button := ButtonBase(c.Key("no-replay")).Width(100).Height(30).Label("No replay")
		button.Children(func() { Text(c, "Click") })
		if button.Clicked() {
			clicks++
		}
	}, 140, 60)
	defer layer.Close()
	layer.Render()
	layer.Render()
	if clicks != 0 {
		t.Fatalf("layer reported %d clicks without input", clicks)
	}
}

func TestUILayerClickDoesNotReplay(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		button := ButtonBase(c.Key("one-click")).Width(100).Height(30).Label("One click")
		button.Children(func() { Text(c, "Click") })
		if button.Clicked() {
			clicks++
		}
	}, 140, 60)
	defer layer.Close()
	layer.HandleInput(InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleInput(InputEvent{Kind: InputPointerUp, X: 20, Y: 20, Button: 0})
	layer.Render()
	layer.Render()
	layer.Render()
	if clicks != 1 {
		t.Fatalf("one click reported %d times", clicks)
	}
}
