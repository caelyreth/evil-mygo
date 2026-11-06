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
	if layer.surfaceButtons != 0 {
		t.Fatal("canceled surface press stayed captured")
	}
	layer.Render()
	if clicks != 0 {
		t.Fatalf("canceled surface press clicked %d times", clicks)
	}
}

func TestUILayerSurfaceInputTracksDragAndResumesInside(t *testing.T) {
	var moves []float32
	layer := NewUILayer(func(c *Context) {
		Box(c).Fill().HandleInput(func(ev InputEvent) bool {
			if ev.Kind == InputPointerMove {
				moves = append(moves, ev.X)
			}
			return true
		})
	}, 100, 60)
	defer layer.Close()
	m := NewPlanarSurface(Rect{W: 100, H: 60})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerDown, X: 10, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerMove, X: 30, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerMove, X: 200, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerMove, X: 50, Y: 20, Button: 0})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerUp, X: 50, Y: 20, Button: 0})
	// A press first moves to its point, then the surface miss holds the
	// last mapped UV and the move back inside resumes the live one.
	if len(moves) != 3 || moves[0] != 10 || absSurface(moves[1]-30) > 1e-5 || moves[2] != 50 {
		t.Fatalf("surface drag moves = %v, want [10 30 50]", moves)
	}
	if layer.surfaceButtons != 0 {
		t.Fatal("surface release stayed captured")
	}
}

func TestUILayerSurfaceInputRejectsBackface(t *testing.T) {
	clicks := 0
	layer := NewUILayer(func(c *Context) {
		button := ButtonBase(c.Key("backface")).Fill().Label("Back")
		if button.Clicked() {
			clicks++
		}
	}, 100, 60)
	defer layer.Close()
	layer.HandleSurfaceInput(backfaceSurface{}, InputEvent{Kind: InputPointerDown, X: 20, Y: 20, Button: 0})
	layer.HandleSurfaceInput(backfaceSurface{}, InputEvent{Kind: InputPointerUp, X: 20, Y: 20, Button: 0})
	layer.Render()
	if clicks != 0 {
		t.Fatalf("back face clicked %d times", clicks)
	}
}

func TestUILayerCursorReflectsMappedHover(t *testing.T) {
	layer := NewUILayer(func(c *Context) {
		Box(c).Size(80, 60).Cursor(CursorPointer).Label("Pointer area")
	}, 100, 60)
	defer layer.Close()
	m := NewPlanarSurface(Rect{W: 100, H: 60})
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerMove, X: 20, Y: 20})
	if got := layer.Cursor(); got != CursorPointer {
		t.Fatalf("layer cursor = %v, want pointer", got)
	}
	layer.HandleSurfaceInput(m, InputEvent{Kind: InputPointerLeave})
	if got := layer.Cursor(); got != CursorDefault {
		t.Fatalf("layer cursor after leave = %v, want default", got)
	}
}

type backfaceSurface struct{}

func (backfaceSurface) Version() uint64 { return 1 }

func (backfaceSurface) ScreenToSurface(Point) (SurfaceHit, bool) {
	return SurfaceHit{UV: UV{U: .2, V: .3}, Backface: true}, true
}

func (backfaceSurface) SurfaceToScreen(UV) (Point, bool) {
	return Point{X: 20, Y: 20}, true
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

func TestUILayerTextInputClientProjectsGeometry(t *testing.T) {
	name := "Ada"
	layer := NewUILayer(func(c *Context) {
		TextInput(c.Key("name"), &name).Fill()
	}, 100, 60)
	defer layer.Close()
	surface := NewPlanarSurface(Rect{X: 100, Y: 50, W: 200, H: 120})
	layer.HandleSurfaceInput(surface, InputEvent{Kind: InputPointerDown, X: 120, Y: 70, Button: 0})
	layer.HandleSurfaceInput(surface, InputEvent{Kind: InputPointerUp, X: 120, Y: 70, Button: 0})
	layer.Render()

	client := layer.TextInputClient(surface)
	if client == nil {
		t.Fatal("layer returned no text input client")
	}
	if got, actual := client.TextForRange(TextInputRange{End: 3}); got != "Ada" || actual != (TextInputRange{End: 3}) {
		t.Fatalf("text range = %q %+v, want Ada {0 3}", got, actual)
	}
	selection := client.Selection()
	innerBounds, _, ok := layer.host.ime.Client.BoundsForRange(selection.Range)
	if !ok {
		t.Fatal("inner text input has no caret bounds")
	}
	bounds, _, ok := client.BoundsForRange(selection.Range)
	if !ok {
		t.Fatal("surface text input has no caret bounds")
	}
	want := Rect{
		X: float32(100 + innerBounds.X/100*200),
		Y: float32(50 + innerBounds.Y/60*120),
		W: float32(innerBounds.W / 100 * 200),
		H: float32(innerBounds.H / 60 * 120),
	}
	if absSurface(bounds.X-want.X) > 1e-4 || absSurface(bounds.Y-want.Y) > 1e-4 ||
		absSurface(bounds.W-want.W) > 1e-4 || absSurface(bounds.H-want.H) > 1e-4 {
		t.Fatalf("projected caret = %+v, want %+v", bounds, want)
	}
	wantIndex, ok := layer.host.ime.Client.IndexForPoint(
		float64(innerBounds.X), float64(innerBounds.Y+innerBounds.H/2),
	)
	if !ok {
		t.Fatal("inner text input rejected its caret point")
	}
	index, ok := client.IndexForPoint(Point{X: want.X, Y: want.Y + want.H/2})
	if !ok || index != wantIndex {
		t.Fatalf("surface index = %d, %v, want %d", index, ok, wantIndex)
	}

	all := TextInputRange{End: 3}
	client.ReplaceText(&all, "Grace")
	layer.Render()
	if name != "Grace" {
		t.Fatalf("bridge replaced text with %q, want Grace", name)
	}
}

func TestUILayerTextInputClientConnectsToOuterHost(t *testing.T) {
	name := "Ada"
	layer := NewUILayer(func(c *Context) {
		TextInput(c.Key("name"), &name).Fill()
	}, 200, 120)
	defer layer.Close()
	surface := NewPlanarSurface(Rect{W: 200, H: 120})
	tt := NewTester(func(c *Context) {
		target := Box(c).Absolute().Left(40).Top(20).Size(200, 120).Focusable().
			HandleTextInput(layer.TextInputClient(surface))
		target.HandleInput(func(ev InputEvent) bool {
			taken := layer.HandleSurfaceInput(surface, ev)
			if taken {
				layer.Render()
			}
			return taken
		})
	}, 400, 200)
	defer tt.rt.close()
	tt.Press(220, 40)
	tt.Release(220, 40)
	layer.Render()
	tt.Frame()
	if active, _ := tt.TextInputActive(); !active {
		t.Fatal("outer host did not activate the layer's text input")
	}
	tt.Type("!")
	layer.Render()
	if name != "Ada!" {
		t.Fatalf("text typed through outer host = %q, want Ada!", name)
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
