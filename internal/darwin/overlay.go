//go:build darwin && !ios

package darwin

import (
	"math"
	"sync"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

type overlayAnimKey struct {
	pulseStart  time.Time
	pulseDelay  time.Duration
	pulsePeriod time.Duration
	pulseMin    float32
	pulseMax    float32
}

type overlayLayer struct {
	clip, fill id
	hasAnim    bool
	animKey    overlayAnimKey
}

const overlayZPosition = 1000.0
const overlayPulseAnimKey = "mygo.pulse"

var (
	overlaySupportOnce sync.Once
	overlaySupportOK   bool
	_                  platform.OverlaySurface = (*surface)(nil)
)

// CompositorOverlaysSupported reports whether Core Animation layers and
// declarative CABasicAnimations are available on this surface.
func (s *surface) CompositorOverlaysSupported() bool {
	if s.w == nil || s.w.closed {
		return false
	}
	overlaySupportOnce.Do(func() {
		overlaySupportOK = hasClass("CALayer") && hasClass("CABasicAnimation") && hasClass("CATransaction")
	})
	return overlaySupportOK
}

// PlaceOverlays updates Core Animation overlay sublayers above the view's
// backing layer and removes any overlays not present in places.
func (s *surface) PlaceOverlays(places []platform.OverlayPlacement) {
	if s.w == nil || s.w.closed {
		return
	}
	if len(places) == 0 && len(s.overlays) == 0 {
		return
	}
	rootLayer := send(s.view, "layer")
	if rootLayer == 0 {
		return
	}
	withPool(func() {
		tx := class("CATransaction")
		send(tx, "begin")
		send(tx, "setDisableActions:", 1)
		defer send(tx, "commit")

		flipped := sendBool(rootLayer, "isGeometryFlipped")
		viewH := msgRect(s.view, sel("bounds")).Size.Height
		scale := s.scale()

		if s.overlays == nil {
			s.overlays = map[uint64]*overlayLayer{}
		}
		activeIDs := make([]uint64, 0, len(places))
		for _, p := range places {
			if p.Clip.W <= 0 || p.Clip.H <= 0 || p.Frame.W <= 0 || p.Frame.H <= 0 {
				continue
			}
			activeIDs = append(activeIDs, p.ID)
			ol := s.overlays[p.ID]
			created := false
			if ol == nil {
				clip := send(send(class("CALayer"), "alloc"), "init")
				fill := send(send(class("CALayer"), "alloc"), "init")
				send(clip, "setMasksToBounds:", 1)
				msgSetFloat(clip, sel("setZPosition:"), overlayZPosition)
				send(clip, "addSublayer:", uintptr(fill))
				send(rootLayer, "addSublayer:", uintptr(clip))
				ol = &overlayLayer{clip: clip, fill: fill}
				s.overlays[p.ID] = ol
				created = true
			}
			clipY := p.Clip.Y
			if !flipped {
				clipY = viewH - p.Clip.Y - p.Clip.H
			}
			msgSetRect(ol.clip, sel("setFrame:"), NSRect{
				Origin: NSPoint{p.Clip.X, clipY},
				Size:   NSSize{p.Clip.W, p.Clip.H},
			})
			msgSetFloat(ol.clip, sel("setContentsScale:"), scale)

			clipFlipped := sendBool(ol.clip, "isGeometryFlipped")
			fillX := p.Frame.X - p.Clip.X
			fillY := p.Frame.Y - p.Clip.Y
			if !clipFlipped {
				fillY = p.Clip.H - (p.Frame.Y - p.Clip.Y) - p.Frame.H
			}
			msgSetRect(ol.fill, sel("setFrame:"), NSRect{
				Origin: NSPoint{fillX, fillY},
				Size:   NSSize{p.Frame.W, p.Frame.H},
			})
			msgSetFloat(ol.fill, sel("setContentsScale:"), scale)
			msgSetFloat(ol.fill, sel("setCornerRadius:"), p.CornerRadius)
			if created && respondsTo(ol.fill, "setCornerCurve:") {
				send(ol.fill, "setCornerCurve:", uintptr(nsString("continuous")))
			}
			cgCol := send(nsColor(p.Color), "CGColor")
			send(ol.fill, "setBackgroundColor:", uintptr(cgCol))

			if p.PulsePeriod > 0 && p.PulseMin != p.PulseMax {
				msgSetFloat32(ol.fill, sel("setOpacity:"), p.PulseMax)
				key := overlayAnimKey{
					pulseStart:  p.PulseStart,
					pulseDelay:  p.PulseDelay,
					pulsePeriod: p.PulsePeriod,
					pulseMin:    p.PulseMin,
					pulseMax:    p.PulseMax,
				}
				if created || !ol.hasAnim || !sameOverlayAnimKey(ol.animKey, key) {
					s.attachOverlayPulse(ol.fill, p)
					ol.animKey = key
					ol.hasAnim = true
				}
			} else {
				if ol.hasAnim {
					send(ol.fill, "removeAnimationForKey:", uintptr(nsString(overlayPulseAnimKey)))
					ol.hasAnim = false
					ol.animKey = overlayAnimKey{}
				}
				op := p.Opacity
				if p.PulsePeriod > 0 {
					op = p.PulseMax
				}
				msgSetFloat32(ol.fill, sel("setOpacity:"), op)
			}
		}

		for id, ol := range s.overlays {
			keep := false
			for _, aid := range activeIDs {
				if aid == id {
					keep = true
					break
				}
			}
			if !keep {
				send(ol.clip, "removeFromSuperlayer")
				release(ol.fill)
				release(ol.clip)
				delete(s.overlays, id)
			}
		}
	})
}

func sameOverlayAnimKey(a, b overlayAnimKey) bool {
	return a.pulseStart.Equal(b.pulseStart) && a.pulseDelay == b.pulseDelay &&
		a.pulsePeriod == b.pulsePeriod && a.pulseMin == b.pulseMin && a.pulseMax == b.pulseMax
}

func (s *surface) attachOverlayPulse(fill id, p platform.OverlayPlacement) {
	anim := send(class("CABasicAnimation"), "animationWithKeyPath:", uintptr(nsString("opacity")))
	fromVal := msgFloatID(class("NSNumber"), sel("numberWithDouble:"), float64(p.PulseMax))
	toVal := msgFloatID(class("NSNumber"), sel("numberWithDouble:"), float64(p.PulseMin))
	send(anim, "setFromValue:", uintptr(fromVal))
	send(anim, "setToValue:", uintptr(toVal))
	msgSetFloat(anim, sel("setDuration:"), p.PulsePeriod.Seconds()/2)
	send(anim, "setAutoreverses:", 1)
	msgSetFloat32(anim, sel("setRepeatCount:"), float32(math.Inf(1)))
	send(anim, "setRemovedOnCompletion:", 0)
	send(anim, "setFillMode:", uintptr(nsString("both")))
	if hasClass("CAMediaTimingFunction") {
		tf := msgTimingCurve(class("CAMediaTimingFunction"), sel("functionWithControlPoints::::"), 0.37, 0.0, 0.63, 1.0)
		if tf != 0 {
			send(anim, "setTimingFunction:", uintptr(tf))
		}
	}
	nowCA := caCurrentMediaTime()
	beginTime := nowCA + p.PulseDelay.Seconds()
	if !p.PulseStart.IsZero() {
		since := time.Since(p.PulseStart)
		if since < p.PulseDelay {
			beginTime = nowCA + (p.PulseDelay - since).Seconds()
		} else {
			elapsed := since - p.PulseDelay
			phaseTime := elapsed % p.PulsePeriod
			beginTime = nowCA - phaseTime.Seconds()
		}
	}
	msgSetFloat(anim, sel("setBeginTime:"), beginTime)
	send(fill, "addAnimation:forKey:", uintptr(anim), uintptr(nsString(overlayPulseAnimKey)))
}

func (s *surface) destroyOverlays() {
	if len(s.overlays) == 0 {
		s.overlays = nil
		return
	}
	for id, ol := range s.overlays {
		send(ol.clip, "removeFromSuperlayer")
		release(ol.fill)
		release(ol.clip)
		delete(s.overlays, id)
	}
	s.overlays = nil
}
