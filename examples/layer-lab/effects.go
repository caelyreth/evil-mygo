package main

import (
	_ "embed"
	"time"

	"github.com/caelyreth/evil-mygo/ui"
	"github.com/caelyreth/evil-mygo/ui/shader"
)

//go:embed optics.kage
var opticsSource string

var (
	optics      = shader.MustCompile(opticsSource)
	uMode       = optics.Uniform("Mode")
	uTime       = optics.Uniform("Time")
	uCenter     = optics.Uniform("Center")
	uStrength   = optics.Uniform("Strength")
	uRadius     = optics.Uniform("Radius")
	uDispersion = optics.Uniform("Dispersion")
)

type animationClock struct {
	elapsed float64
	since   time.Time
	running bool
}

func (c *animationClock) at(now time.Time) float32 {
	t := c.elapsed
	if c.running && !c.since.IsZero() {
		t += max(now.Sub(c.since).Seconds(), 0)
	}
	return float32(t)
}

func (c *animationClock) setRunning(now time.Time, running bool) {
	c.elapsed = float64(c.at(now))
	c.since, c.running = now, running
}

// layerEffect updates uniforms at paint time, so animation does not need
// to rebuild the view. Cache keeps the unchanged source layer on the GPU.
type layerEffect struct {
	app    *app
	values *shader.Values
}

func newLayerEffect(a *app) *layerEffect { return &layerEffect{app: a, values: optics.NewValues()} }

func (*layerEffect) EffectMargin() ui.Point { return ui.Point{} }

func (fx *layerEffect) PaintEffect(p *ui.Painter, box ui.Rect, content *ui.Bitmap, inset ui.Point) {
	a := fx.app
	if a.bypass || a.strength == 0 || !p.Accelerated() || p.Preferences().ReduceTransparency {
		// The CPU interpreter is useful for small tests, not a full-window
		// animated preview. Keep the actual controls usable without a GPU.
		s := p.Scale()
		p.ImageRegion(content, ui.Rect{X: inset.X, Y: inset.Y, W: box.W * s, H: box.H * s}, box, ui.ImageOptions{})
		return
	}
	fx.values.Int(uMode, int32(a.mode)).Float(uTime, a.clock.at(p.Now())).
		Vec2(uCenter, a.center.X*box.W, a.center.Y*box.H).
		Float(uStrength, float32(a.strength)).Float(uRadius, float32(a.radius)).
		Float(uDispersion, float32(a.dispersion))
	optics.Effect(shader.Options{Values: fx.values}).PaintEffect(p, box, content, inset)
	if a.clock.running {
		p.AnimationFrameRate(60)
	}
}
