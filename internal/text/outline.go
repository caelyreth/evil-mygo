package text

// PathOp is the kind of a step of a glyph's outline.
type PathOp uint8

const (
	PathMove PathOp = iota
	PathLine
	PathQuad
	PathCubic
	PathClose
)

// PathSeg is a step of a glyph's outline: its control points, then its
// end point, in DIPs from the glyph's origin, y up.
type PathSeg struct {
	Op  PathOp
	Pts [3][2]float32
}

// GlyphPath returns the outline of glyph id of f at f's size, as its
// curves, nil where the platform's text system gives none.
func GlyphPath(f *Font, id uint32) []PathSeg {
	if f == nil {
		return nil
	}
	return glyphPath(f, id)
}
