package main

import (
	"math"
	"testing"
)

func TestArchiveIdentifiers(t *testing.T) {
	if archiveFileCount != 33280 {
		t.Fatalf("catalogue has %d files", archiveFileCount)
	}
	seen := make(map[int]bool, archiveFileCount)
	for i := 0; i < archiveFileCount; i++ {
		cell := cellAt(i)
		if !cell.valid() || cell.index() != i || seen[cell.number()] {
			t.Fatalf("invalid or duplicate identity: %+v", cell)
		}
		seen[cell.number()] = true
	}
	if (archiveCell{2, 12}).number() != 1 {
		t.Fatal("the initial file changed identity")
	}
}

func TestArchiveNavigationCaps(t *testing.T) {
	for _, cell := range []archiveCell{{minLane, minRow}, {minLane, maxRow}, {maxLane, minRow}, {maxLane, maxRow}} {
		c := newSceneController()
		c.clock.elapsed = playbackDuration
		c.selectCell(cell)
		settleController(t, c)
		minX, maxX, minZ, maxZ := cameraLimits()
		if x, z := float64(c.current.aim.x), float64(c.current.aim.z); x < minX || x > maxX || z < minZ || z > maxZ {
			t.Fatalf("camera focus escaped the catalogue at %+v: %+v", cell, c.current.aim)
		}
		lamp := c.selectionLamp()
		if x, z := float64(lamp[0]), float64(lamp[2]); x < minX || x > maxX || z < minZ || z > maxZ {
			t.Fatalf("selection lamp escaped the catalogue at %+v: %+v", cell, lamp)
		}
		c.dirty = false
		lane, row := -1, -1
		if cell.lane == maxLane {
			lane = 1
		}
		if cell.row == maxRow {
			row = 1
		}
		for i := 0; i < 100; i++ {
			c.navigate(lane, row)
		}
		if c.selected != cell || c.active || c.dirty {
			t.Fatal("a boundary press restarted motion or changed the selection")
		}
		c.selectCell(archiveCell{cell.lane + lane, cell.row + row})
		if c.selected != cell || c.dirty {
			t.Fatal("picking can select outside the cap")
		}
		maxInt := int(^uint(0) >> 1)
		c.navigate(maxInt, maxInt)
		if c.selected != (archiveCell{maxLane, maxRow}) {
			t.Fatal("large positive navigation wrapped around")
		}
		c.navigate(-maxInt, -maxInt)
		if c.selected != (archiveCell{minLane, minRow}) {
			t.Fatal("large negative navigation wrapped around")
		}
	}
}

func TestArchiveGuardOccludesButCannotBePicked(t *testing.T) {
	c := newSceneController()
	c.hasShown, c.shownAspect = true, 16.0/9
	c.shownCamera = sceneCamera{eye: vec3{0, 1.8, 10}, aim: vec3{0, 1.8, 0}, forward: vec3{0, 0, -1}, span: 8, distance: 10}
	c.shown = []shownCard{
		{archiveCell{2, 12}, instance{position: [4]float32{0, 0, 0, 0}, rotation: [4]float32{0, 1}}},
		{archiveCell{maxLane + 1, 12}, instance{position: [4]float32{0, 0, 2, 0}, rotation: [4]float32{0, 1}}},
	}
	if cell, hit := c.pick(800, 450, 1600, 900); hit {
		t.Fatalf("picked through the non-selectable guard: %+v", cell)
	}
}

func TestRollingArchiveHasBoundedResidency(t *testing.T) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	c.activate()
	var result []instance
	for _, cell := range []archiveCell{{2, 12}, {10, 70}, {-10, -50}, {maxLane, maxRow}, {minLane, minRow}, {2, 12}} {
		c.selectCell(cell)
		settleController(t, c)
		var camera sceneCamera
		result, camera, _ = c.snapshot(result, 16.0/9)
		if len(result) == 0 || len(result) > residentCapacity || len(c.cards) > residentCapacity+2 {
			t.Fatalf("unbounded render/simulation pool: %d instances, %d states", len(result), len(c.cards))
		}
		window := windowAt(camera, float32(c.entry.value))
		selected := false
		for _, shown := range c.shown {
			if !shown.cell.rendered() || !window.contains(shown.cell) {
				t.Fatalf("residency outside its guard: %+v", shown.cell)
			}
			selected = selected || shown.cell == cell
		}
		if !selected {
			t.Fatalf("selected file disappeared at %+v", cell)
		}
		if window.minLane >= cell.lane || window.maxLane <= cell.lane || window.minRow >= cell.row || window.maxRow <= cell.row {
			t.Fatal("no rendered margin around the navigation cap")
		}
		for _, pose := range result {
			for _, v := range pose.position {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Fatal("invalid streamed card transform")
				}
			}
		}
	}
}

func TestDossierIDHashKeepsStableVariedFamilies(t *testing.T) {
	var families [4]int
	for i := range 256 {
		cell := cellAt(i)
		hash := cell.dossierHash()
		if hash != cellAt(cell.index()).dossierHash() {
			t.Fatal("dossier design changed for the same file identity")
		}
		families[hash&3]++
	}
	for family, count := range families {
		if count < 24 {
			t.Fatalf("dossier family %d is missing from the neighboring file sample", family)
		}
	}
}

func BenchmarkArchiveFrame(b *testing.B) {
	c := newSceneController()
	c.clock.elapsed = playbackDuration
	instances, _, _ := c.snapshot(nil, 16.0/9)
	c.selectCell(archiveCell{2, 13})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.time += 1.0 / 60
		c.step(1.0 / 60)
		instances, _, _ = c.snapshot(instances, 16.0/9)
	}
}
