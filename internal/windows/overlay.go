//go:build windows && (amd64 || arm64)

package windows

import (
	"log"
	"math"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

type overlaySurfaceKey struct {
	w, h   int32
	offX   float32
	offY   float32
	frameW float32
	frameH float32
	radius float32
	color  platform.Color
}

type overlayAnimKey struct {
	pulseStart  time.Time
	pulseDelay  time.Duration
	pulsePeriod time.Duration
	pulseMin    float32
	pulseMax    float32
}

type overlayItem struct {
	visual, surface, effect, anim uintptr
	offX, offY                    float32
	hasOffset                     bool
	surfKey                       overlaySurfaceKey
	hasSurf                       bool
	animKey                       overlayAnimKey
	hasAnim                       bool
	opacity                       float32
	hasOpacity                    bool
	hasEffect                     bool
}

type overlayTree struct {
	dev          *composition
	target, root uintptr
	items        map[uint64]*overlayItem
	order        []uint64
	px           []uint32
}

var _ platform.OverlaySurface = (*surface)(nil)

// CompositorOverlaysSupported reports whether DirectComposition overlays and
// declarative opacity animations are supported on this surface.
func (s *surface) CompositorOverlaysSupported() bool {
	if s.w == nil || s.w.closed || s.viaComposition() || s.w.b == nil {
		return false
	}
	return s.w.b.composition() != nil
}

// PlaceOverlays updates DirectComposition visual overlays above the surface
// and removes any overlays not present in places.
func (s *surface) PlaceOverlays(places []platform.OverlayPlacement) {
	if s.w == nil || s.w.closed || s.viaComposition() || s.w.b == nil {
		return
	}
	s.lastOverlays = append(s.lastOverlays[:0], places...)
	if s.overlays == nil && len(places) == 0 {
		s.overlaysLost = false
		return
	}
	b := s.w.b
	dev := b.composition()
	if dev == nil {
		if s.overlays != nil {
			s.freeOverlays()
		}
		if len(s.lastOverlays) > 0 {
			s.overlaysLost = true
			b.composeLater(b.compRetry)
		}
		return
	}
	if err := s.placeOverlays(dev, places); err != nil {
		s.freeOverlays()
		s.overlaysLost = len(s.lastOverlays) > 0
		if dev.lost() {
			b.loseComposition(err)
			return
		}
		log.Printf("mygo: cannot place DirectComposition overlays: %v", err)
		b.composeLater(time.Now().Add(time.Second))
		return
	}
	b.compFails = 0
	s.overlaysLost = false
}

func (s *surface) placeOverlays(dev *composition, places []platform.OverlayPlacement) error {
	if s.overlays != nil && s.overlays.dev != dev {
		s.freeOverlays()
	}
	validCount := 0
	for _, p := range places {
		if validOverlayPlacement(p) {
			validCount++
		}
	}
	if s.overlays == nil && validCount == 0 {
		return nil
	}
	tree := s.overlays
	treeCreated := false
	if tree == nil {
		var target, root uintptr
		if hr := comCall(dev.dcomp, dcompDevCreateTargetForHwnd, s.hwnd, 1, uintptr(unsafe.Pointer(&target))); failed(hr) {
			return hresultError("IDCompositionDevice::CreateTargetForHwnd", hr)
		}
		if hr := comCall(dev.dcomp, dcompDevCreateVisual, uintptr(unsafe.Pointer(&root))); failed(hr) {
			release(target)
			return hresultError("IDCompositionDevice::CreateVisual", hr)
		}
		if hr := comCall(target, dcompTargetSetRoot, root); failed(hr) {
			release(root)
			release(target)
			return hresultError("IDCompositionTarget::SetRoot", hr)
		}
		tree = &overlayTree{
			dev:    dev,
			target: target,
			root:   root,
			items:  map[uint64]*overlayItem{},
		}
		s.overlays = tree
		treeCreated = true
	}

	scale := float64(s.dpi()) / 96.0
	if scale <= 0 {
		scale = 1
	}

	dirty := treeCreated
	activeIDs := make([]uint64, 0, validCount)
	now := time.Now()
	for _, p := range places {
		if !validOverlayPlacement(p) {
			continue
		}
		activeIDs = append(activeIDs, p.ID)

		ol := tree.items[p.ID]
		itemCreated := false
		if ol == nil {
			var visual uintptr
			if hr := comCall(dev.dcomp, dcompDevCreateVisual, uintptr(unsafe.Pointer(&visual))); failed(hr) {
				return hresultError("IDCompositionDevice::CreateVisual", hr)
			}
			ol = &overlayItem{visual: visual}
			tree.items[p.ID] = ol
			itemCreated = true
			dirty = true
		}

		cx := snapNearInt(p.Clip.X * scale)
		cy := snapNearInt(p.Clip.Y * scale)
		cr := snapNearInt((p.Clip.X + p.Clip.W) * scale)
		cb := snapNearInt((p.Clip.Y + p.Clip.H) * scale)
		clipX := int32(math.Floor(cx))
		clipY := int32(math.Floor(cy))
		clipR := int32(math.Ceil(cr))
		clipB := int32(math.Ceil(cb))
		if clipR <= clipX {
			clipR = clipX + 1
		}
		if clipB <= clipY {
			clipB = clipY + 1
		}
		w := clipR - clipX
		h := clipB - clipY

		offX := float32(clipX)
		offY := float32(clipY)
		if !ol.hasOffset || ol.offX != offX || ol.offY != offY {
			if hr := comCallFloat32(ol.visual, dcompVisualSetOffsetXFloat, offX); failed(hr) {
				return hresultError("IDCompositionVisual::SetOffsetX", hr)
			}
			if hr := comCallFloat32(ol.visual, dcompVisualSetOffsetYFloat, offY); failed(hr) {
				return hresultError("IDCompositionVisual::SetOffsetY", hr)
			}
			ol.offX, ol.offY = offX, offY
			ol.hasOffset = true
			dirty = true
		}

		fx := snapNearInt(p.Frame.X * scale)
		fy := snapNearInt(p.Frame.Y * scale)
		fw := snapNearInt(p.Frame.W * scale)
		fh := snapNearInt(p.Frame.H * scale)
		surfKey := overlaySurfaceKey{
			w:      w,
			h:      h,
			offX:   float32(fx - float64(clipX)),
			offY:   float32(fy - float64(clipY)),
			frameW: float32(fw),
			frameH: float32(fh),
			radius: float32(p.CornerRadius * scale),
			color:  p.Color,
		}
		if itemCreated || !ol.hasSurf || ol.surfKey != surfKey {
			if err := tree.updateSurface(dev, ol, surfKey); err != nil {
				return err
			}
			ol.surfKey = surfKey
			ol.hasSurf = true
			dirty = true
		}

		changedOpacity, err := updateOverlayOpacity(dev, ol, p, now)
		if err != nil {
			return err
		}
		if changedOpacity {
			dirty = true
		}
	}

	if treeCreated || !sameUint64Slice(tree.order, activeIDs) {
		if hr := comCall(tree.root, dcompVisualRemoveAllVisuals); failed(hr) {
			return hresultError("IDCompositionVisual::RemoveAllVisuals", hr)
		}
		for _, id := range activeIDs {
			if hr := comCall(tree.root, dcompVisualAddVisual, tree.items[id].visual, 0, 0); failed(hr) {
				return hresultError("IDCompositionVisual::AddVisual", hr)
			}
		}
		tree.order = append(tree.order[:0], activeIDs...)
		dirty = true
	}

	for id, ol := range tree.items {
		if !containsUint64(activeIDs, id) {
			ol.free()
			delete(tree.items, id)
			dirty = true
		}
	}

	if dirty {
		if hr := comCall(dev.dcomp, dcompDevCommit); failed(hr) {
			return hresultError("IDCompositionDevice::Commit", hr)
		}
	}
	return nil
}

func (t *overlayTree) updateSurface(dev *composition, ol *overlayItem, key overlaySurfaceKey) error {
	if ol.surface == 0 || !ol.hasSurf || ol.surfKey.w != key.w || ol.surfKey.h != key.h {
		release(ol.surface)
		ol.surface = 0
		if hr := comCall(dev.dcomp, dcompDevCreateSurface, uintptr(key.w), uintptr(key.h), dxgiFormatB8G8R8A8, dxgiAlphaPremultiplied,
			uintptr(unsafe.Pointer(&ol.surface))); failed(hr) {
			return hresultError("IDCompositionDevice::CreateSurface", hr)
		}
		if hr := comCall(ol.visual, dcompVisualSetContent, ol.surface); failed(hr) {
			return hresultError("IDCompositionVisual::SetContent", hr)
		}
	}

	n := int(key.w) * int(key.h)
	if cap(t.px) < n {
		t.px = make([]uint32, n)
	}
	px := t.px[:n]
	rasterRoundedRect(px, key)

	var tex uintptr
	var offset point
	if hr := comCall(ol.surface, dcompSurfaceBeginDraw, 0, uintptr(unsafe.Pointer(&iidID3D11Texture2D)),
		uintptr(unsafe.Pointer(&tex)), uintptr(unsafe.Pointer(&offset))); failed(hr) {
		return hresultError("IDCompositionSurface::BeginDraw", hr)
	}
	box := [6]uint32{uint32(offset.X), uint32(offset.Y), 0, uint32(offset.X + key.w), uint32(offset.Y + key.h), 1}
	comCall(dev.ctx, d3dCtxUpdateSubresource, tex, 0, uintptr(unsafe.Pointer(&box)), uintptr(unsafe.Pointer(&px[0])), uintptr(key.w*4), 0)
	release(tex)
	if hr := comCall(ol.surface, dcompSurfaceEndDraw); failed(hr) {
		return hresultError("IDCompositionSurface::EndDraw", hr)
	}
	return nil
}

func updateOverlayOpacity(dev *composition, ol *overlayItem, p platform.OverlayPlacement, now time.Time) (bool, error) {
	animKey := overlayAnimKey{
		pulseStart:  p.PulseStart,
		pulseDelay:  p.PulseDelay,
		pulsePeriod: p.PulsePeriod,
		pulseMin:    p.PulseMin,
		pulseMax:    p.PulseMax,
	}
	if p.PulsePeriod > 0 && p.PulseMin != p.PulseMax {
		if ol.hasAnim && sameOverlayAnimKey(ol.animKey, animKey) {
			return false, nil
		}
		if err := ensureOverlayEffect(dev, ol); err != nil {
			return false, err
		}
		if ol.anim == 0 {
			if hr := comCall(dev.dcomp, dcompDevCreateAnimation, uintptr(unsafe.Pointer(&ol.anim))); failed(hr) {
				return false, hresultError("IDCompositionDevice::CreateAnimation", hr)
			}
		} else {
			if hr := comCall(ol.anim, dcompAnimReset); failed(hr) {
				return false, hresultError("IDCompositionAnimation::Reset", hr)
			}
		}

		minA, maxA := p.PulseMin, p.PulseMax
		bias := (minA + maxA) * 0.5
		amp := (maxA - minA) * 0.5
		freq := float32(1.0 / p.PulsePeriod.Seconds())

		var since time.Duration
		if !p.PulseStart.IsZero() {
			since = now.Sub(p.PulseStart)
			if since < 0 {
				since = 0
			}
		}
		if since < p.PulseDelay {
			remain := (p.PulseDelay - since).Seconds()
			if remain > 1e-4 {
				if hr := comCallAnimSeg(ol.anim, dcompAnimAddCubic, 0, maxA, 0, 0, 0); failed(hr) {
					return false, hresultError("IDCompositionAnimation::AddCubic", hr)
				}
				if hr := comCallAnimSeg(ol.anim, dcompAnimAddSinusoidal, remain, bias, amp, freq, 90); failed(hr) {
					return false, hresultError("IDCompositionAnimation::AddSinusoidal", hr)
				}
			} else {
				if hr := comCallAnimSeg(ol.anim, dcompAnimAddSinusoidal, 0, bias, amp, freq, 90); failed(hr) {
					return false, hresultError("IDCompositionAnimation::AddSinusoidal", hr)
				}
			}
		} else {
			elapsed := since - p.PulseDelay
			phaseDeg := float32(90.0 + math.Mod(elapsed.Seconds()/p.PulsePeriod.Seconds(), 1.0)*360.0)
			if hr := comCallAnimSeg(ol.anim, dcompAnimAddSinusoidal, 0, bias, amp, freq, phaseDeg); failed(hr) {
				return false, hresultError("IDCompositionAnimation::AddSinusoidal", hr)
			}
		}

		if hr := comCall(ol.effect, dcompEffectSetOpacityAnim, ol.anim); failed(hr) {
			return false, hresultError("IDCompositionEffectGroup::SetOpacity(anim)", hr)
		}
		ol.hasAnim = true
		ol.hasOpacity = false
		ol.animKey = animKey
		return true, nil
	}

	op := p.Opacity
	if p.PulsePeriod > 0 {
		op = p.PulseMax
	}
	if op >= 1 && !ol.hasAnim && !ol.hasEffect {
		ol.opacity = 1
		ol.hasOpacity = true
		return false, nil
	}
	if !ol.hasAnim && ol.hasOpacity && ol.opacity == op {
		return false, nil
	}
	if err := ensureOverlayEffect(dev, ol); err != nil {
		return false, err
	}
	if hr := comCallFloat32(ol.effect, dcompEffectSetOpacityFloat, op); failed(hr) {
		return false, hresultError("IDCompositionEffectGroup::SetOpacity(float)", hr)
	}
	ol.hasAnim = false
	ol.animKey = overlayAnimKey{}
	ol.opacity = op
	ol.hasOpacity = true
	return true, nil
}

func ensureOverlayEffect(dev *composition, ol *overlayItem) error {
	if ol.effect == 0 {
		if hr := comCall(dev.dcomp, dcompDevCreateEffectGroup, uintptr(unsafe.Pointer(&ol.effect))); failed(hr) {
			return hresultError("IDCompositionDevice::CreateEffectGroup", hr)
		}
	}
	if !ol.hasEffect {
		if hr := comCall(ol.visual, dcompVisualSetEffect, ol.effect); failed(hr) {
			return hresultError("IDCompositionVisual::SetEffect", hr)
		}
		ol.hasEffect = true
	}
	return nil
}

func rasterRoundedRect(px []uint32, k overlaySurfaceKey) {
	w := int(k.w)
	h := int(k.h)
	hx := float64(k.frameW) * 0.5
	hy := float64(k.frameH) * 0.5
	cx := float64(k.offX) + hx
	cy := float64(k.offY) + hy
	r := math.Min(float64(k.radius), math.Min(hx, hy))
	if r < 0 {
		r = 0
	}
	ix := math.Max(0, hx-r)
	iy := math.Max(0, hy-r)
	colA := float64(k.color.A) / 255.0
	colR := float64(k.color.R)
	colG := float64(k.color.G)
	colB := float64(k.color.B)

	for py := 0; py < h; py++ {
		dy := math.Abs(float64(py)+0.5-cy) - iy
		row := py * w
		for pxIdx := 0; pxIdx < w; pxIdx++ {
			dx := math.Abs(float64(pxIdx)+0.5-cx) - ix
			dist := math.Hypot(math.Max(dx, 0), math.Max(dy, 0)) + math.Min(math.Max(dx, dy), 0) - r
			cov := 0.5 - dist
			if cov <= 0 {
				px[row+pxIdx] = 0
				continue
			}
			if cov > 1 {
				cov = 1
			}
			alpha := colA * cov
			cr := uint8(math.Round(colR * alpha))
			cg := uint8(math.Round(colG * alpha))
			cb := uint8(math.Round(colB * alpha))
			ca := uint8(math.Round(255.0 * alpha))
			px[row+pxIdx] = uint32(cb) | uint32(cg)<<8 | uint32(cr)<<16 | uint32(ca)<<24
		}
	}
}

func (s *surface) freeOverlays() {
	if s.overlays == nil {
		return
	}
	s.overlays.free()
	s.overlays = nil
}

func (t *overlayTree) free() {
	for id, ol := range t.items {
		ol.free()
		delete(t.items, id)
	}
	for _, p := range []*uintptr{&t.root, &t.target} {
		if *p != 0 {
			release(*p)
			*p = 0
		}
	}
	t.dev = nil
	t.order = nil
}

func (ol *overlayItem) free() {
	for _, p := range []*uintptr{&ol.anim, &ol.effect, &ol.surface, &ol.visual} {
		if *p != 0 {
			release(*p)
			*p = 0
		}
	}
}

func validOverlayPlacement(p platform.OverlayPlacement) bool {
	return p.Clip.W > 0 && p.Clip.H > 0 && p.Frame.W > 0 && p.Frame.H > 0
}

func snapNearInt(v float64) float64 {
	r := math.Round(v)
	if math.Abs(v-r) < 1e-3 {
		return r
	}
	return v
}

func sameOverlayAnimKey(a, b overlayAnimKey) bool {
	return a.pulseStart.Equal(b.pulseStart) && a.pulseDelay == b.pulseDelay &&
		a.pulsePeriod == b.pulsePeriod && a.pulseMin == b.pulseMin && a.pulseMax == b.pulseMax
}

func sameUint64Slice(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsUint64(s []uint64, v uint64) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
