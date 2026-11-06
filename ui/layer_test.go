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
