package main

import (
	"math"
	"slices"
	"testing"
)

func TestGeneratedArchiveModel(t *testing.T) {
	m := newArchiveModel()
	if len(m.meshes) != 5 || len(m.vertices) == 0 || len(m.indices) == 0 {
		t.Fatalf("incomplete model: %d meshes, %d vertices, %d indices", len(m.meshes), len(m.vertices), len(m.indices))
	}
	array, glass := 0, 0
	for _, mesh := range m.meshes {
		switch mesh.name {
		case "Frosted_Polymer", "Frosted_Polymer_Back", "Optical_Diffuser", "Titanium_Fasteners", "Index_Inlay":
		default:
			t.Fatalf("unexpected physical mesh group: %s", mesh.name)
		}
		if mesh.array {
			array++
		}
		if mesh.glass {
			glass++
		}
	}
	if array != 5 || glass != 2 {
		t.Fatalf("surface categories: %d array, %d glass", array, glass)
	}
	for _, index := range m.indices {
		if int(index) >= len(m.vertices) {
			t.Fatal("vertex index outside model")
		}
	}
	for _, vertex := range m.vertices {
		p, n := vertex.position, vertex.normal
		if p[0] < -2.6 || p[0] > 2.6 || p[1] < -.1 || p[1] > 3.8 || p[2] < -.5 || p[2] > .5 {
			t.Fatalf("unexpected model coordinates: %v", p)
		}
		if math.Abs(float64(n[0]*n[0]+n[1]*n[1]+n[2]*n[2]-1)) > .001 {
			t.Fatalf("non-unit normal: %v", n)
		}
	}
}

func TestEveryEntryHasTouchingEqualThicknessLayers(t *testing.T) {
	m := newArchiveModel()
	type bounds struct{ lo, hi float32 }
	layers := make(map[string]bounds)
	for _, part := range m.meshes {
		if part.name != "Frosted_Polymer" && part.name != "Frosted_Polymer_Back" && part.name != "Optical_Diffuser" {
			continue
		}
		if !part.array || part.glass != (part.name != "Optical_Diffuser") {
			t.Fatalf("layer does not render on every entry with the right material: %s", part.name)
		}
		b := bounds{lo: float32(math.Inf(1)), hi: float32(math.Inf(-1))}
		for _, index := range m.indices[part.first : part.first+part.count] {
			z := m.vertices[index].position[2]
			b.lo, b.hi = min(b.lo, z), max(b.hi, z)
		}
		if math.Abs(float64(b.hi-b.lo)-.14) > .00001 {
			t.Fatalf("%s is not a substantial equal-thickness layer: %+v", part.name, b)
		}
		if part.glass && math.Abs(float64(m.materials[part.material].optics[2])-.14) > .00001 {
			t.Fatalf("%s optical thickness disagrees with its geometry", part.name)
		}
		minX, maxX, minY, maxY := float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.Inf(1)), float32(math.Inf(-1))
		for _, index := range m.indices[part.first : part.first+part.count] {
			p := m.vertices[index].position
			minX, maxX, minY, maxY = min(minX, p[0]), max(maxX, p[0]), min(minY, p[1]), max(maxY, p[1])
		}
		if math.Abs(float64(minX+2.5)) > .00001 || math.Abs(float64(maxX-2.5)) > .00001 ||
			math.Abs(float64(minY)) > .00001 || math.Abs(float64(maxY)-3.7) > .00001 {
			t.Fatalf("%s does not share the laminate footprint", part.name)
		}
		layers[part.name] = b
	}
	if len(layers) != 3 {
		t.Fatalf("found %d physical layers, want three", len(layers))
	}
	front, core, back := layers["Frosted_Polymer"], layers["Optical_Diffuser"], layers["Frosted_Polymer_Back"]
	if math.Abs(float64(back.hi-core.lo)) > .00001 || math.Abs(float64(core.hi-front.lo)) > .00001 {
		t.Fatalf("laminate faces do not touch: %+v", layers)
	}
	if math.Abs(float64(front.lo+back.hi)) > .00001 || math.Abs(float64(front.hi+back.lo)) > .00001 {
		t.Fatal("outer layers do not form a symmetric laminate")
	}
}

func TestFastenersHaveMirroredEdgeInsets(t *testing.T) {
	m := newArchiveModel()
	lo := [2][2]float32{{100, 100}, {100, 100}}
	hi := [2][2]float32{{-100, -100}, {-100, -100}}
	for _, part := range m.meshes {
		if part.name != "Titanium_Fasteners" {
			continue
		}
		for _, index := range m.indices[part.first : part.first+part.count] {
			p := m.vertices[index].position
			side := 0
			if p[0] > 0 {
				side = 1
			}
			for axis := range 2 {
				lo[side][axis], hi[side][axis] = min(lo[side][axis], p[axis]), max(hi[side][axis], p[axis])
			}
		}
	}
	for axis, total := range [2]float32{0, 3.7} {
		left, right := (lo[0][axis]+hi[0][axis])/2, (lo[1][axis]+hi[1][axis])/2
		if math.Abs(float64(left+right-total)) > .00001 {
			t.Fatalf("fastener centers are not mirrored on axis %d: %v, %v", axis, left, right)
		}
	}
}

func TestArchiveGeometryIsDeterministic(t *testing.T) {
	a, b := newArchiveModel(), newArchiveModel()
	if !slices.Equal(a.vertices, b.vertices) || !slices.Equal(a.indices, b.indices) ||
		!slices.Equal(a.materials, b.materials) || !slices.Equal(a.meshes, b.meshes) {
		t.Fatal("procedural geometry changes between openings")
	}
	for _, v := range a.vertices {
		for _, x := range v.position {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				t.Fatal("procedural geometry contains a non-finite position")
			}
		}
	}
}

func TestGeneratedFastenerHasARecessedCross(t *testing.T) {
	outline := fastenerSlot(0)
	if len(outline) != 36 {
		t.Fatal("cross recess lost its eight rounded and four inside corners")
	}
	for _, angle := range []float64{.22, .22 + math.Pi/2, .22 + math.Pi} {
		if math.Abs(outlineRadius(outline, angle)-.0345) > .000001 {
			t.Fatal("unexpected screw recess arm length")
		}
	}
	if radius := outlineRadius(outline, .22+math.Pi/4); math.Abs(radius-.007*math.Sqrt2) > .000001 {
		t.Fatal("cross recess was filled in or replaced by a circular hole")
	}
}

func TestScenePlacement(t *testing.T) {
	camera := cameraAt(shotEnd, 16.0/9)
	instances := sceneInstances(nil, shotEnd, camera.forward)
	if len(instances) != 11*51 {
		t.Fatalf("instance count = %d", len(instances))
	}
	selected := instances[len(instances)-1]
	if selected.position[3] != 1 || math.Abs(float64(selected.position[1])-(-4.6+selectedField(0, 0)+.4)) > .00001 {
		t.Fatalf("raised card = %+v", selected)
	}
	for _, instance := range instances[:len(instances)-1] {
		if instance.position[0] == selected.position[0] && instance.position[2] == selected.position[2] {
			t.Fatal("selected card also appears in the instanced array")
		}
	}
	for _, size := range [][2]int{{1280, 800}, {800, 1280}} {
		camera := cameraAt(shotEnd, float32(size[0])/float32(size[1]))
		g := sceneGlobals(size[0], size[1], shotEnd, camera)
		p := point(g.viewProjection, camera.aim)
		if math.Abs(float64(p.x)) > .001 || math.Abs(float64(p.y)) > .001 {
			t.Fatalf("camera aim is not centered: %+v", p)
		}
	}
}
