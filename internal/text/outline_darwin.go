package text

import (
	"slices"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The curves of a glyph's outline, as CGPathApply hands its elements to
// the one callback made for it; pathMu serializes the readers.
var (
	pathMu   sync.Mutex
	pathOnce sync.Once
	pathFn   uintptr
	pathSegs []PathSeg
)

func glyphPath(f *Font, id uint32) []PathSeg {
	if f.native == 0 {
		return nil
	}
	pathMu.Lock()
	defer pathMu.Unlock()
	pathOnce.Do(func() {
		pathFn = purego.NewCallback(func(_ uintptr, el *cgPathElement) uintptr {
			seg := PathSeg{}
			n := 0
			switch el.kind {
			case 0:
				seg.Op, n = PathMove, 1
			case 1:
				seg.Op, n = PathLine, 1
			case 2:
				seg.Op, n = PathQuad, 2
			case 3:
				seg.Op, n = PathCubic, 3
			case 4:
				seg.Op = PathClose
			}
			if n > 0 {
				pts := unsafe.Slice(el.points, n)
				for i, p := range pts {
					seg.Pts[i] = [2]float32{float32(p.x), float32(p.y)}
				}
			}
			pathSegs = append(pathSegs, seg)
			return 0
		})
	})
	pathSegs = pathSegs[:0]
	path := ct.fontCreatePath(f.native, uint16(id), 0)
	if path == 0 {
		return nil
	}
	ct.pathApply(path, 0, pathFn)
	ct.pathRelease(path)
	return slices.Clone(pathSegs)
}
