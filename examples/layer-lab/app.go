package main

import (
	"fmt"
	"math"

	"github.com/caelyreth/evil-mygo/ui"
)

const sidebarWidth = 230

var (
	paper = ui.Hex("#f5f2eb")
	ink   = ui.Hex("#253a3b")
	muted = ui.Hex("#747b72")
	line  = ui.Hex("#dcded3")
	rust  = ui.Hex("#c75130")
)

var modes = [...]struct{ name, subtitle, description string }{
	{"Crystal lens", "Refraction + dispersion", "A rounded glass lens magnifies the live tree, bends its edges and separates the spectrum. Move across the specimen to inspect the pixels."},
	{"Fluted glass", "Ribbed distortion + frost", "Vertical glass ribs bend the same native text, artwork and controls. A small multi-tap frost softens the transmitted image."},
	{"Liquid ripple", "Radial displacement", "Concentric waves displace the entire component. The pointer is the wave source; chromatic separation catches the moving crests."},
	{"Prism echo", "Facets + chromatic ghosts", "Diagonal facets split the native layer into offset spectral samples. No second UI tree is built for the colored echoes."},
	{"Mosaic focus", "Quantized sampling", "The outside becomes a grid of sampled colors while a clear circular lens preserves the original detail. Same tree, different sampling coordinates."},
}

type app struct {
	theme                        *ui.Theme
	mode                         int
	strength, radius, dispersion float64
	animate, follow, bypass      bool
	center                       ui.Point
	clock                        animationClock
	fx                           *layerEffect
	title                        string
	progress                     float64
	playing, saved               bool
}

func newApp() *app {
	t := ui.LightTheme()
	t.Background, t.Text, t.TextMuted = paper, ink, muted
	t.Surface, t.SurfaceHover, t.Border = ui.Hex("#fffdf8"), ui.Hex("#e9e8de"), line
	t.Accent, t.AccentHover, t.AccentPressed = rust, ui.Hex("#ad4025"), ui.Hex("#91351f")
	t.InactiveAccent, t.Selection, t.Focus = rust, ui.RGBA(199, 81, 48, 0.18), ui.RGBA(199, 81, 48, 0.55)
	t.Font, t.Radius = "Avenir Next", 8
	a := &app{
		theme: t, strength: 0.75, radius: 0.58, dispersion: 0.5,
		animate: true, follow: true, center: ui.Point{X: 0.56, Y: 0.47},
		title: "Evening currents", progress: 0.38,
	}
	a.fx = newLayerEffect(a)
	return a
}

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	ui.Column(c).Fill().Padding(24, 28, 18, 28).Gap(22).Children(func() {
		ui.Row(c).Height(30).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(12, 12).Radius(6).Border(3, rust).Margin(0, 10, 0, 0)
			label(c, "MYGO / NATIVE EXPERIMENTS", ink)
			ui.Spacer(c)
			label(c, "OPTICS LAB     /     001", muted)
		})
		ui.Box(c).Height(1).Background(line)
		ui.Row(c).Grow(1).MinHeight(0).Gap(28).AlignItems(ui.Stretch).Children(func() {
			ui.Scroll(c).Width(sidebarWidth).Shrink(0).Children(func() { a.sidebar(c) })
			a.preview(c)
		})
		ui.Row(c).AlignItems(ui.Center).Gap(18).Children(func() {
			label(c, "GO TREE  ->  GPU LAYER  ->  FRAGMENT SHADER", muted)
			ui.Spacer(c)
			label(c, "NO WEBVIEW. NO SCREENSHOTS.", rust)
		})
	})
	// Change the clock's running state after the controls applied their input.
	a.clock.setRunning(c.Now(), a.animate && !a.bypass && a.strength > 0 && !c.Preferences().ReduceMotion && !c.Preferences().ReduceTransparency)
}

func label(c *ui.Context, text string, color ui.Color) {
	ui.Text(c, text).Font("monospace").FontSize(10).TextColor(color).NoWrap()
}

func (a *app) sidebar(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(12).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, "Light, bent.").Font("Baskerville").FontSize(42).TextColor(ink)
			ui.Text(c, "One native component.\nFive ways to look through it.").FontSize(13).LineHeight(1.5).TextColor(muted)
		})
		ui.Column(c).Gap(2).Children(func() {
			for i, mode := range modes {
				b := ui.ButtonBase(c.Key(fmt.Sprintf("mode-%d", i))).FillWidth().Height(38).
					Justify(ui.Start).Padding(0, 12).Gap(10).Radius(8).Label(mode.name).Role(ui.RoleButton).Selected(a.mode == i)
				color := muted
				if a.mode == i {
					b.Background(ui.Hex("#e9e7dc"))
					color = rust
				} else if b.Hovered() {
					b.Background(ui.Hex("#eeece3"))
				}
				b.Children(func() {
					label(c, fmt.Sprintf("%02d", i+1), color)
					ui.Text(c, mode.name).FontSize(14).FontWeight(600).TextColor(ink)
				})
				if b.Clicked() {
					a.mode = i
				}
			}
		})
		ui.Box(c).Height(1).Background(line)
		a.parameter(c, "Strength", "strength", &a.strength)
		a.parameter(c, "Lens size", "radius", &a.radius)
		a.parameter(c, "Dispersion", "dispersion", &a.dispersion)
		ui.Column(c).Gap(6).Children(func() {
			ui.Checkbox(c.Key("follow"), &a.follow, "Follow pointer").FontSize(12)
			ui.Checkbox(c.Key("animate"), &a.animate, "Animate light").FontSize(12)
			ui.Checkbox(c.Key("bypass"), &a.bypass, "Show original").FontSize(12)
		})
		if ui.Button(c.Key("reset"), "Reset experiment").FillWidth().Clicked() {
			a.mode, a.strength, a.radius, a.dispersion = 0, 0.75, 0.58, 0.5
			a.follow, a.animate, a.bypass = true, true, false
			a.center = ui.Point{X: 0.56, Y: 0.47}
			a.clock = animationClock{}
		}
	})
}

func (a *app) parameter(c *ui.Context, name, key string, value *float64) {
	ui.Column(c).Gap(6).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, name).FontSize(12).TextColor(muted)
			ui.Spacer(c)
			label(c, fmt.Sprintf("%03.0f", *value*100), ink)
		})
		ui.Slider(c.Key(key), value, 0, 1).FillWidth().Label(name)
	})
}

func (a *app) preview(c *ui.Context) {
	w, _ := c.Size()
	cardWidth := min(float32(620), w-2*28-sidebarWidth-28-64)
	ui.Column(c).Grow(1).MinWidth(0).Gap(14).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			label(c, "THE SPECIMEN", muted)
			ui.Spacer(c)
			label(c, "LIVE NATIVE UI", rust)
		})
		ui.Box(c).Grow(1).MinHeight(390).Radius(20).Clip().Border(1, line).Children(func() {
			stage := ui.Box(c.Key("specimen")).Fill().Label("Native specimen").Cache().Effect(a.fx).Draw(drawStage)
			if a.follow {
				if x, y, over := stage.PointerPosition(); over {
					r := stage.Bounds()
					if r.W > 0 && r.H > 0 {
						a.center = ui.Point{X: clamp(x/r.W, 0, 1), Y: clamp(y/r.H, 0, 1)}
					}
				}
			}
			stage.Children(func() {
				ui.Box(c).Absolute().Top(22).Left(24).Children(func() { label(c, "FIG. 01 / A LISTENING ROOM", ink) })
				ui.Column(c).Fill().Center().Padding(54, 32).Children(func() { a.player(c, cardWidth) })
				ui.Box(c).Absolute().Bottom(22).Left(24).Children(func() { label(c, "TEXTINPUT   /   SLIDER   /   BUTTON", muted) })
			})
		})
		ui.Column(c).Gap(6).MinHeight(100).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(14).Children(func() {
				ui.Text(c, modes[a.mode].name).Font("Baskerville").FontSize(29)
				label(c, modes[a.mode].subtitle, rust)
			})
			ui.Text(c, modes[a.mode].description).FontSize(12).LineHeight(1.5).TextColor(muted)
			ui.Text(c, "Pixels bend; hit targets do not. Pause or show the original to edit without distortion.").FontSize(11).TextColor(rust)
		})
	})
}

func (a *app) player(c *ui.Context, width float32) {
	ui.Row(c).Width(width).Height(286).Radius(16).Clip().Background(ui.Hex("#fffdf7")).
		Shadow(0, 10, 28, 0, ui.RGBA(36, 58, 50, 0.14)).Children(func() {
		ui.Box(c).Width(width * 0.39).FillHeight().Shrink(0).Draw(drawCover).Children(func() {
			ui.Column(c).Fill().Justify(ui.End).Padding(20).Gap(4).Children(func() {
				label(c, "FIELD RECORDINGS", ui.Hex("#f9edda"))
				ui.Text(c, "Tidal\nStudies").Font("Baskerville").FontSize(38).LineHeight(0.95).TextColor(ui.Hex("#fff5e3"))
				label(c, "VOL. 04 / STEREO", ui.Hex("#e6bd98"))
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Padding(22).Gap(13).Children(func() {
			label(c, "NOW PLAYING / SIDE A", muted)
			ui.Text(c, "A quieter\nkind of noise.").Font("Baskerville").FontSize(30).LineHeight(1.05)
			ui.TextInput(c.Key("track-title"), &a.title).Label("Track title").FontSize(12).
				Padding(7, 9).Background(ui.Hex("#f3f2ea")).Border(0, ui.Transparent).FillWidth()
			ui.Column(c).Gap(3).Children(func() {
				ui.Slider(c.Key("position"), &a.progress, 0, 1).FillWidth().Label("Track position")
				ui.Row(c).Children(func() {
					label(c, fmt.Sprintf("%02d:%02d", int(a.progress*248)/60, int(a.progress*248)%60), muted)
					ui.Spacer(c)
					label(c, "04:08", muted)
				})
			})
			ui.Row(c).Gap(8).Children(func() {
				play := "Play"
				if a.playing {
					play = "Pause"
				}
				if ui.PrimaryButton(c.Key("play"), play).Label("Toggle playback").Width(78).Clicked() {
					a.playing = !a.playing
				}
				save := "Save track"
				if a.saved {
					save = "Saved"
				}
				if ui.Button(c.Key("save"), save).Label("Save track").Grow(1).MinWidth(0).Clicked() {
					a.saved = !a.saved
				}
			})
		})
	})
}

func drawStage(p *ui.Painter, r ui.Rect) {
	p.FillGradient(r, ui.LinearGradient{From: ui.Hex("#e8eade"), To: ui.Hex("#f1dfc9"), Angle: 135}, 0)
	for x := r.X + 18; x < r.X+r.W; x += 26 {
		for y := r.Y + 18; y < r.Y+r.H; y += 26 {
			p.Fill(ui.Rect{X: x, Y: y, W: 1.5, H: 1.5}, ui.RGBA(63, 85, 69, 0.15), 0.75)
		}
	}
	d := min(r.W*0.72, r.H*1.15)
	arc := ui.Rect{X: r.X + r.W*0.62 - d/2, Y: r.Y + r.H*0.36 - d/2, W: d, H: d}
	p.Stroke(arc, ui.Hex("#d2af83"), d/2, 1)
	arc.X, arc.Y, arc.W, arc.H = arc.X+18, arc.Y+18, d-36, d-36
	p.Stroke(arc, ui.Hex("#d2af83"), arc.W/2, 1)
}

func drawCover(p *ui.Painter, r ui.Rect) {
	p.FillGradient(r, ui.LinearGradient{From: ui.Hex("#16585c"), To: ui.Hex("#172f37"), Angle: 170}, 0)
	d := r.W * 0.74
	p.Fill(ui.Rect{X: r.X + r.W*0.15, Y: r.Y + 28, W: d, H: d}, ui.Hex("#eea96e"), d/2)
	for i := range 14 {
		y := r.Y + 70 + float32(i)*10
		var wave ui.Path
		for j := range 33 {
			x := r.W * float32(j) / 32
			py := y + float32(math.Sin(float64(x/r.W*6.2+float32(i)*0.22)))*13
			if j == 0 {
				wave.MoveTo(r.X+x, py)
			} else {
				wave.LineTo(r.X+x, py)
			}
		}
		p.StrokePath(&wave, 3.5, ui.Hex("#246b69"))
	}
}

func clamp(v, lo, hi float32) float32 { return min(max(v, lo), hi) }
