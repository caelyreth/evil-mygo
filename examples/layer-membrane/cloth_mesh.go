//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import "github.com/egoist/mygo/ui"

const (
	clothColumns = 120
	clothRows    = 80
	clothTile    = 8
	clothHalfW   = panelHalfH * layerWidth / layerHeight
	nearPlane    = 0.03
	farPlane     = 80
)

type clothVertex struct {
	position v3
	normal   v3
	tangent  v3
	u, v     float32
}

type clothBounds struct {
	lo, hi v3
}

type clothMesh struct {
	vertices  []clothVertex
	indices   []uint32
	tiles     []clothBounds
	waveTime  float32
	amplitude float32
	version   uint64
}

func (s *membraneSurface) geometry() *clothMesh {
	if s.mesh == nil {
		s.mesh = &clothMesh{
			vertices: make([]clothVertex, (clothColumns+1)*(clothRows+1)),
			indices:  make([]uint32, 0, clothColumns*clothRows*6),
			tiles:    make([]clothBounds, clothColumns/clothTile*clothRows/clothTile),
		}
		for y := range clothRows {
			for x := range clothColumns {
				i := uint32(y*(clothColumns+1) + x)
				below := i + clothColumns + 1
				s.mesh.indices = append(s.mesh.indices, i, below, i+1, i+1, below, below+1)
			}
		}
	}
	m := s.mesh
	if m.version != 0 && m.waveTime == s.waveTime && m.amplitude == s.amplitude {
		return m
	}
	m.waveTime, m.amplitude = s.waveTime, s.amplitude
	m.rebuild()
	m.version++
	return m
}

func (m *clothMesh) rebuild() {
	const stride = clothColumns + 1
	const step = float32(2 * clothHalfW / clothColumns)
	t, amplitude := m.waveTime, m.amplitude
	for row := range clothRows + 1 {
		v := float32(row) / clothRows
		x, z := float32(-clothHalfW), float32(0)
		for column := range clothColumns + 1 {
			u := float32(column) / clothColumns
			if column > 0 {
				mid := u - 0.5/clothColumns
				envelope := mid * (0.28 + 0.72*mid)
				angle := amplitude * (envelope*(0.68*sin32(9*mid-2.2*t+1.1*v)+
					0.20*sin32(18*mid-3.6*t-2*v)+
					0.075*sin32(30*mid-5.3*t+3*v)) +
					0.18*mid*(0.65+0.35*sin32(0.8*t-v)))
				// Integrating unit tangents preserves each strip's length as it folds.
				x += step * cos32(angle)
				z += step * sin32(angle)
			}
			envelope := u * u
			y := panelLift + panelHalfH - 2*panelHalfH*v + amplitude*envelope*(0.045*sin32(5.5*u-1.4*t+2*v)+
				0.018*sin32(19*u-3.8*t+4*v)-0.10*(0.3+0.7*v))
			m.vertices[row*stride+column] = clothVertex{position: v3{x, y, z}, u: u, v: v}
		}
	}
	for row := range clothRows + 1 {
		for column := range clothColumns + 1 {
			du := sub3(m.vertices[row*stride+min(column+1, clothColumns)].position,
				m.vertices[row*stride+max(column-1, 0)].position)
			dv := sub3(m.vertices[min(row+1, clothRows)*stride+column].position,
				m.vertices[max(row-1, 0)*stride+column].position)
			vertex := &m.vertices[row*stride+column]
			vertex.normal = normalize3(cross3(dv, du))
			vertex.tangent = normalize3(du)
		}
	}
	for ty := range clothRows / clothTile {
		for tx := range clothColumns / clothTile {
			p := m.vertices[ty*clothTile*stride+tx*clothTile].position
			bounds := clothBounds{lo: p, hi: p}
			for y := ty * clothTile; y <= (ty+1)*clothTile; y++ {
				for x := tx * clothTile; x <= (tx+1)*clothTile; x++ {
					p := m.vertices[y*stride+x].position
					bounds.lo = v3{min(bounds.lo.X, p.X), min(bounds.lo.Y, p.Y), min(bounds.lo.Z, p.Z)}
					bounds.hi = v3{max(bounds.hi.X, p.X), max(bounds.hi.Y, p.Y), max(bounds.hi.Z, p.Z)}
				}
			}
			m.tiles[ty*(clothColumns/clothTile)+tx] = bounds
		}
	}
}

func (m *clothMesh) point(uv ui.UV) (v3, v3) {
	x := min(int(uv.U*clothColumns), clothColumns-1)
	y := min(int(uv.V*clothRows), clothRows-1)
	u, v := uv.U*clothColumns-float32(x), uv.V*clothRows-float32(y)
	i := (y*clothColumns + x) * 6
	weights := [3]float32{1 - u - v, v, u}
	if u+v > 1 {
		i += 3
		weights = [3]float32{1 - v, 1 - u, u + v - 1}
	}
	a, b, c := m.vertices[m.indices[i]].position, m.vertices[m.indices[i+1]].position, m.vertices[m.indices[i+2]].position
	p := add3(scale3(a, weights[0]), add3(scale3(b, weights[1]), scale3(c, weights[2])))
	return p, cross3(sub3(b, a), sub3(c, a))
}

func (s *membraneSurface) intersect(cam, rd v3) (surfaceHit, bool) {
	m := s.geometry()
	_, forward, _, _ := s.cameraBasis()
	viewDepth := dot3(rd, forward)
	if viewDepth <= 0 {
		return surfaceHit{}, false
	}
	closest := float32(farPlane) / viewDepth
	if floor, ok := s.groundDepth(cam, rd); ok && floor*viewDepth >= nearPlane {
		closest = min(closest, floor)
	}
	var hit surfaceHit
	found := false
	ray := [3]float64{float64(rd.X), float64(rd.Y), float64(rd.Z)}
	for tile, bounds := range m.tiles {
		if !bounds.intersects(cam, rd, closest) {
			continue
		}
		tx, ty := tile%(clothColumns/clothTile), tile/(clothColumns/clothTile)
		for y := ty * clothTile; y < (ty+1)*clothTile; y++ {
			for x := tx * clothTile; x < (tx+1)*clothTile; x++ {
				for triangle := range 2 {
					i := (y*clothColumns+x)*6 + triangle*3
					a, b, c := m.vertices[m.indices[i]], m.vertices[m.indices[i+1]], m.vertices[m.indices[i+2]]
					// Float32 cancellation can open cracks at shared mesh vertices.
					// Keep intersection arithmetic in float64, not just its result.
					edge1, edge2 := sub64(b.position, a.position), sub64(c.position, a.position)
					p := cross64(ray, edge2)
					det := dot64(edge1, p)
					if det > -1e-12 && det < 1e-12 {
						continue
					}
					rel := sub64(cam, a.position)
					u := dot64(rel, p) / det
					q := cross64(rel, edge1)
					v := dot64(ray, q) / det
					if u < -1e-10 || v < -1e-10 || u+v > 1+1e-10 {
						continue
					}
					t := float32(dot64(edge2, q) / det)
					if t*viewDepth < nearPlane || t >= closest {
						continue
					}
					closest, found = t, true
					hit = surfaceHit{
						t: t, u: a.u + (b.u-a.u)*float32(u) + (c.u-a.u)*float32(v),
						v: a.v + (b.v-a.v)*float32(u) + (c.v-a.v)*float32(v), front: det > 0,
					}
				}
			}
		}
	}
	return hit, found
}

func sub64(a, b v3) [3]float64 {
	return [3]float64{float64(a.X) - float64(b.X), float64(a.Y) - float64(b.Y), float64(a.Z) - float64(b.Z)}
}

func cross64(a, b [3]float64) [3]float64 {
	return [3]float64{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}

func dot64(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

func (b clothBounds) intersects(origin, direction v3, limit float32) bool {
	near, far := float32(0), limit
	for _, axis := range [3][4]float32{
		{origin.X, direction.X, b.lo.X, b.hi.X},
		{origin.Y, direction.Y, b.lo.Y, b.hi.Y},
		{origin.Z, direction.Z, b.lo.Z, b.hi.Z},
	} {
		if abs32(axis[1]) < 1e-8 {
			if axis[0] < axis[2]-1e-6 || axis[0] > axis[3]+1e-6 {
				return false
			}
			continue
		}
		a, c := (axis[2]-axis[0])/axis[1], (axis[3]-axis[0])/axis[1]
		near, far = max(near, min(a, c)), min(far, max(a, c))
		if near > far+1e-6 {
			return false
		}
	}
	return true
}
