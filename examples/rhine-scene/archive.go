package main

import "math"

const (
	minLane          = -30
	maxLane          = 34
	minRow           = -244
	maxRow           = 267
	archiveRows      = maxRow - minRow + 1
	archiveColumns   = maxLane - minLane + 1
	archiveFileCount = archiveRows * archiveColumns

	// Keep a rolling field around the rendered camera instead of submitting
	// the entire catalogue. Non-selectable guard cards conceal its outer edge.
	residentLanes    = 21
	residentRows     = 193
	residentCapacity = residentLanes * residentRows
	laneGuard        = residentLanes / 2
	rowGuard         = residentRows / 2
)

type archiveCell struct{ lane, row int }

func (c archiveCell) index() int { return (c.lane-minLane)*archiveRows + c.row - minRow }
func cellAt(index int) archiveCell {
	return archiveCell{minLane + index/archiveRows, minRow + index%archiveRows}
}
func (c archiveCell) valid() bool {
	return c.lane >= minLane && c.lane <= maxLane && c.row >= minRow && c.row <= maxRow
}
func (c archiveCell) rendered() bool {
	return c.lane >= minLane-laneGuard && c.lane <= maxLane+laneGuard && c.row >= minRow-rowGuard && c.row <= maxRow+rowGuard
}
func (c archiveCell) number() int {
	n := (c.lane-2)*archiveRows + c.row - 12
	return 1 + (n%archiveFileCount+archiveFileCount)%archiveFileCount
}

func (c archiveCell) dossierHash() uint32 {
	// A stable avalanche hash, not runtime randomness or neighboring-ID modulo.
	x := uint32(c.number())
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	return x ^ (x >> 16)
}
func (c archiveCell) position() (x, z float32) {
	return float32(c.lane-2) * 5.2, (float32(c.row) - 15.5) * .62
}

type archiveWindow struct{ minLane, maxLane, minRow, maxRow int }

func windowAt(camera sceneCamera, entry float32) archiveWindow {
	lane := min(max(int(math.Round(float64(camera.aim.x/5.2+2))), minLane), maxLane)
	row := min(max(int(math.Round(float64((camera.aim.z-entry)/.62+15.5))), minRow), maxRow)
	return archiveWindow{lane - laneGuard, lane + laneGuard, row - rowGuard, row + rowGuard}
}

func (w archiveWindow) contains(c archiveCell) bool {
	return c.lane >= w.minLane && c.lane <= w.maxLane && c.row >= w.minRow && c.row <= w.maxRow
}

func cameraLimits() (minX, maxX, minZ, maxZ float64) {
	x, z := (archiveCell{minLane, minRow}).position()
	x1, z1 := (archiveCell{maxLane, maxRow}).position()
	return float64(x), float64(x1), float64(z), float64(z1)
}
