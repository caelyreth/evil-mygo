package main

import (
	"math"
	"slices"
)

// Build all five physical mesh groups without external model data.
// Third-party notices for portions of this construction are in THIRD_PARTY_LICENSES.
func newArchiveModel() *model {
	m := &model{vertices: make([]vertex, 0, 4096), indices: make([]uint32, 0, 16000)}
	polymer := material{color: [4]float32{.995, .99, .975, .22}, optics: [4]float32{0, .97, .14, 1}}
	core := material{color: [4]float32{.80, .74, .64, .66}, optics: [4]float32{0, 0, 0, 2}}
	titanium := material{color: [4]float32{.58, .60, .61, .19}, optics: [4]float32{.82}}
	inlay := material{color: [4]float32{.67, .53, .40, .52}, optics: [4]float32{.04, 0, 0, 4}}
	for _, layer := range []struct {
		name   string
		lo, hi float64
		mat    material
		glass  bool
	}{
		{"Frosted_Polymer", .07, .21, polymer, true},
		{"Frosted_Polymer_Back", -.21, -.07, polymer, true},
		{"Optical_Diffuser", -.07, .07, core, false},
	} {
		part := m.beginMesh(layer.name, layer.mat, layer.glass)
		part.plate(vec3{}, 2.5, 3.7, .04, .008, layer.lo, layer.hi)
		part.finish()
	}
	marker := m.beginMesh("Index_Inlay", inlay, false)
	marker.plate(vec3{-2.158977, 3.4760969, 0}, .125, .2239031, .001, .0008, .202, .206)
	marker.finish()
	fasteners := m.beginMesh("Titanium_Fasteners", titanium, false)
	fasteners.fastener(vec3{-2.3345037, .13993955, 0})
	fasteners.fastener(vec3{2.3345037, 3.5600605, 0})
	fasteners.finish()
	return m
}

type geometryBuilder struct {
	m        *model
	mesh     int
	material int
}

func (m *model) beginMesh(name string, mat material, glass bool) geometryBuilder {
	index := len(m.materials)
	m.materials = append(m.materials, mat)
	m.meshes = append(m.meshes, mesh{name: name, first: uint32(len(m.indices)), material: index, array: true, glass: glass})
	return geometryBuilder{m: m, mesh: len(m.meshes) - 1, material: index}
}

func (b geometryBuilder) finish() {
	part := &b.m.meshes[b.mesh]
	part.count = uint32(len(b.m.indices)) - part.first
}

func (b geometryBuilder) vertex(p, n vec3) uint32 {
	i := uint32(len(b.m.vertices))
	n = unit(n)
	b.m.vertices = append(b.m.vertices, vertex{position: [4]float32{p.x, p.y, p.z, 1},
		normal: [4]float32{n.x, n.y, n.z, float32(b.material)}})
	return i
}

func (b geometryBuilder) triangle(a, c, d uint32) {
	pa, pc, pd := b.m.vertices[a], b.m.vertices[c], b.m.vertices[d]
	p := func(v vertex) vec3 { return vec3{v.position[0], v.position[1], v.position[2]} }
	n := vec3{pa.normal[0] + pc.normal[0] + pd.normal[0], pa.normal[1] + pc.normal[1] + pd.normal[1], pa.normal[2] + pc.normal[2] + pd.normal[2]}
	if dot(cross(sub(p(pc), p(pa)), sub(p(pd), p(pa))), n) < 0 {
		c, d = d, c
	}
	b.m.indices = append(b.m.indices, a, c, d)
}

func (b geometryBuilder) join(a, c []uint32) {
	for i := range a {
		j := (i + 1) % len(a)
		b.triangle(a[i], a[j], c[j])
		b.triangle(a[i], c[j], c[i])
	}
}

// Rounded rectangular rings form three bevel/sidewall bands with closed ends.
func (b geometryBuilder) plate(origin vec3, halfWidth, height, corner, bevel, lo, hi float64) {
	ring := func(inset, z, radial, axial float64) []uint32 {
		result := make([]uint32, 0, 28)
		centers := [4][2]float64{{halfWidth - corner, corner}, {halfWidth - corner, height - corner},
			{-halfWidth + corner, height - corner}, {-halfWidth + corner, corner}}
		for i, center := range centers {
			for step := range 7 {
				angle := float64(i*90-90+step*15) * math.Pi / 180
				nx, ny := math.Cos(angle), math.Sin(angle)
				p := add(origin, vec3{float32(center[0] + nx*(corner-inset)), float32(center[1] + ny*(corner-inset)), float32(z)})
				result = append(result, b.vertex(p, vec3{float32(nx * radial), float32(ny * radial), float32(axial)}))
			}
		}
		return result
	}
	diagonal := math.Sqrt(.5)
	b.join(ring(bevel, lo, diagonal, -diagonal), ring(0, lo+bevel, diagonal, -diagonal))
	b.join(ring(0, lo+bevel, 1, 0), ring(0, hi-bevel, 1, 0))
	b.join(ring(0, hi-bevel, diagonal, diagonal), ring(bevel, hi, diagonal, diagonal))
	for _, end := range [][2]float64{{lo, -1}, {hi, 1}} {
		face := ring(bevel, end[0], 0, end[1])
		center := b.vertex(add(origin, vec3{0, float32(height / 2), float32(end[0])}), vec3{0, 0, float32(end[1])})
		for i, a := range face {
			b.triangle(center, a, face[(i+1)%len(face)])
		}
	}
}

type outlinePoint struct{ x, y float64 }

// The screw's cross recess has eight rounded outside corners and four sharp
// inside corners, recessed beneath the head with a 0.22-radian rotation.
func fastenerSlot(inset float64) []outlinePoint {
	length, width, radius := .0345-inset, .007-inset, max(.001-inset, 0)
	outline := []outlinePoint{{length, -width}, {length, width}, {width, width}, {width, length},
		{-width, length}, {-width, width}, {-length, width}, {-length, -width},
		{-width, -width}, {-width, -length}, {width, -length}, {width, -width}}
	result := make([]outlinePoint, 0, 36)
	for i, v := range outline {
		prev, next := outline[(i+len(outline)-1)%len(outline)], outline[(i+1)%len(outline)]
		ax, ay, bx, by := prev.x-v.x, prev.y-v.y, next.x-v.x, next.y-v.y
		al, bl := math.Hypot(ax, ay), math.Hypot(bx, by)
		ax, ay, bx, by = ax/al, ay/al, bx/bl, by/bl
		if ax*by-ay*bx < 0 && radius > 0 {
			cx, cy := v.x+(ax+bx)*radius, v.y+(ay+by)*radius
			start := math.Atan2(-by, -bx)
			for step := range 4 {
				a := start + float64(step)*math.Pi/6
				result = append(result, outlinePoint{cx + radius*math.Cos(a), cy + radius*math.Sin(a)})
			}
		} else {
			result = append(result, v)
		}
	}
	for i, p := range result {
		result[i] = outlinePoint{p.x*math.Cos(.22) - p.y*math.Sin(.22), p.x*math.Sin(.22) + p.y*math.Cos(.22)}
	}
	return result
}

func outlineRadius(outline []outlinePoint, angle float64) float64 {
	dx, dy := math.Cos(angle), math.Sin(angle)
	radius := math.Inf(1)
	for i, a := range outline {
		c := outline[(i+1)%len(outline)]
		ex, ey := c.x-a.x, c.y-a.y
		denominator := dx*ey - dy*ex
		if math.Abs(denominator) < 1e-12 {
			continue
		}
		t, u := (a.x*ey-a.y*ex)/denominator, (a.x*dy-a.y*dx)/denominator
		if t > 0 && u >= -1e-8 && u <= 1+1e-8 {
			radius = min(radius, t)
		}
	}
	return radius
}

func (b geometryBuilder) fastener(origin vec3) {
	outline := fastenerSlot(0)
	angles := make([]float64, 0, 64+len(outline)*2)
	for i := range 64 {
		angles = append(angles, float64(i)*2*math.Pi/64)
	}
	for _, boundary := range [][]outlinePoint{outline, fastenerSlot(.001)} {
		for _, p := range boundary {
			angles = append(angles, math.Mod(math.Atan2(p.y, p.x)+2*math.Pi, 2*math.Pi))
		}
	}
	slices.Sort(angles)
	angles = slices.CompactFunc(angles, func(a, c float64) bool { return math.Abs(a-c) < 1e-8 })
	pointAt := func(angle, radius, z float64) vec3 {
		return add(origin, vec3{float32(math.Cos(angle) * radius), float32(math.Sin(angle) * radius), float32(z)})
	}
	ring := func(radius, z, radial, axial float64) []uint32 {
		indices := make([]uint32, 0, len(angles))
		for _, a := range angles {
			indices = append(indices, b.vertex(pointAt(a, radius, z), vec3{float32(math.Cos(a) * radial), float32(math.Sin(a) * radial), float32(axial)}))
		}
		return indices
	}
	const radius, edgeInset, bevel = .06, .0030065, .006
	var bottom, previous, top []uint32
	for _, end := range []struct{ z, sign float64 }{{.192, -1}, {.2, 1}} {
		for step := range 4 {
			angle := float64(step) * math.Pi / 6
			if end.sign > 0 {
				angle = math.Pi/2 - angle
			}
			r := radius - edgeInset + edgeInset*math.Sin(angle)
			z := end.z + end.sign*bevel*math.Cos(angle)
			nr, nz := math.Sin(angle)/edgeInset, end.sign*math.Cos(angle)/bevel
			current := ring(r, z, nr, nz)
			if previous != nil {
				b.join(previous, current)
			} else {
				bottom = current
			}
			previous, top = current, current
		}
	}
	base := b.vertex(add(origin, vec3{z: .186}), vec3{z: -1})
	for i, a := range bottom {
		b.triangle(base, a, bottom[(i+1)%len(bottom)])
	}
	slotRing := func(inset, z float64) []uint32 {
		boundary := fastenerSlot(inset)
		indices := make([]uint32, 0, len(angles))
		for _, a := range angles {
			indices = append(indices, b.vertex(pointAt(a, outlineRadius(boundary, a), z), vec3{z: 1}))
		}
		return indices
	}
	upper := slotRing(0, .206)
	b.join(top, upper)
	// Duplicate cavity edge vertices: vertical walls must not inherit the
	// upward face normals, and the closed recess floor must remain visible.
	cavityBand := func(a, c []uint32) {
		p := func(index uint32) vec3 {
			v := b.m.vertices[index].position
			return vec3{v[0], v[1], v[2]}
		}
		for i := range a {
			j := (i + 1) % len(a)
			p0, p1, p2, p3 := p(a[i]), p(a[j]), p(c[j]), p(c[i])
			n := unit(cross(sub(p1, p0), sub(p3, p0)))
			ids := [4]uint32{b.vertex(p0, n), b.vertex(p1, n), b.vertex(p2, n), b.vertex(p3, n)}
			b.triangle(ids[0], ids[1], ids[2])
			b.triangle(ids[0], ids[2], ids[3])
		}
	}
	lower := slotRing(0, .2)
	cavityBand(upper, lower)
	for step := 1; step <= 3; step++ {
		a := float64(step) * math.Pi / 6
		next := slotRing(.001*(1-math.Cos(a)), .2-.002*math.Sin(a))
		cavityBand(lower, next)
		lower = next
	}
	floor := b.vertex(add(origin, vec3{z: .198}), vec3{z: 1})
	for i, a := range lower {
		b.triangle(floor, a, lower[(i+1)%len(lower)])
	}
}
