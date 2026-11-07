package ui

import "github.com/caelyreth/evil-mygo/internal/platform"

// layerTextClient adapts an off-window layer's native text input to the
// element that presents it.
type layerTextClient struct {
	layer   *UILayer
	surface SurfaceMap
	version uint64
}

func (c *layerTextClient) client() platform.TextInputClient {
	if c == nil || c.layer == nil || c.layer.closed {
		return nil
	}
	return c.layer.host.ime.Client
}

func (c *layerTextClient) TextForRange(r TextInputRange) (string, TextInputRange) {
	if client := c.client(); client != nil {
		return client.TextForRange(r)
	}
	return "", TextInputRange{}
}

func (c *layerTextClient) Selection() TextInputSelection {
	if client := c.client(); client != nil {
		return client.Selection()
	}
	return TextInputSelection{}
}

func (c *layerTextClient) MarkedRange() (TextInputRange, bool) {
	if client := c.client(); client != nil {
		return client.MarkedRange()
	}
	return TextInputRange{}, false
}

func (c *layerTextClient) ReplaceText(r *TextInputRange, text string) {
	if client := c.client(); client != nil {
		client.ReplaceText(r, text)
	}
}

func (c *layerTextClient) SetMarkedText(r *TextInputRange, text string, selected TextInputRange) {
	if client := c.client(); client != nil {
		client.SetMarkedText(r, text, selected)
	}
}

func (c *layerTextClient) UnmarkText() {
	if client := c.client(); client != nil {
		client.UnmarkText()
	}
}

func (c *layerTextClient) BoundsForRange(r TextInputRange) (Rect, TextInputRange, bool) {
	client := c.client()
	if client == nil || c.surface == nil || c.surface.Version() != c.version {
		return Rect{}, TextInputRange{}, false
	}
	bounds, actual, ok := client.BoundsForRange(r)
	if !ok {
		return Rect{}, actual, false
	}
	screen, ok := c.layerRectToSurface(Rect{
		X: float32(bounds.X), Y: float32(bounds.Y),
		W: float32(bounds.W), H: float32(bounds.H),
	})
	return screen, actual, ok
}

func (c *layerTextClient) IndexForPoint(p Point) (int, bool) {
	client := c.client()
	if client == nil || c.surface == nil || c.surface.Version() != c.version {
		return 0, false
	}
	hit, ok := c.surface.ScreenToSurface(p)
	if !ok || hit.Backface {
		return 0, false
	}
	return client.IndexForPoint(
		float64(hit.UV.U*c.layer.host.w),
		float64(hit.UV.V*c.layer.host.h),
	)
}

func (c *layerTextClient) layerRectToSurface(r Rect) (Rect, bool) {
	w, h := c.layer.host.w, c.layer.host.h
	if w <= 0 || h <= 0 {
		return Rect{}, false
	}
	uvs := [4]UV{
		{U: r.X / w, V: r.Y / h},
		{U: (r.X + r.W) / w, V: r.Y / h},
		{U: (r.X + r.W) / w, V: (r.Y + r.H) / h},
		{U: r.X / w, V: (r.Y + r.H) / h},
	}
	var minX, minY, maxX, maxY float32
	for i, uv := range uvs {
		p, ok := c.surface.SurfaceToScreen(uv)
		if !ok {
			return Rect{}, false
		}
		if i == 0 {
			minX, minY, maxX, maxY = p.X, p.Y, p.X, p.Y
			continue
		}
		minX, minY = min(minX, p.X), min(minY, p.Y)
		maxX, maxY = max(maxX, p.X), max(maxY, p.Y)
	}
	return Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}
