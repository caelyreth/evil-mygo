// Package packed describes the byte layout of shader uniforms used by the
// byte-addressable WebGPU storage buffer and by the other language
// backends that share this packed layout.
package packed

import (
	"fmt"

	"github.com/egoist/mygo/internal/shaderir"
)

// UniformLayout returns where each of p's uniform variables lies in the
// packed uniform buffer, in bytes, and the buffer's size: a vector of 3
// takes the room of 4, a matrix is its columns, and an array's elements
// are as far apart as their alignment makes them.
func UniformLayout(p *shaderir.Program) (offsets []int, size int) {
	align := 1
	for i := range p.Uniforms {
		s, a := sizeAlign(&p.Uniforms[i])
		size = roundUp(size, a)
		offsets = append(offsets, size)
		size += s
		align = max(align, a)
	}
	return offsets, roundUp(size, align)
}

// UniformLanes returns, for each lane of p's uniforms as package interp
// keeps them (in order, columns of matrices, elements of arrays), its
// offset in bytes in the packed uniform buffer, and the buffer's size.
func UniformLanes(p *shaderir.Program) (lanes []int, size int) {
	offsets, size := UniformLayout(p)
	for i := range p.Uniforms {
		for _, at := range TypeLanes(&p.Uniforms[i]) {
			lanes = append(lanes, offsets[i]+at)
		}
	}
	return lanes, size
}

// TypeLanes returns the byte offsets of a value's lanes in the packed
// uniform layout.
func TypeLanes(t *shaderir.Type) []int {
	var out []int
	var walk func(t *shaderir.Type, base int)
	walk = func(t *shaderir.Type, base int) {
		switch {
		case t.Main == shaderir.Array:
			s, a := sizeAlign(&t.Sub[0])
			stride := roundUp(s, a)
			for i := range t.Length {
				walk(&t.Sub[0], base+i*stride)
			}
		case t.IsMatrix():
			n := t.MatrixSize()
			col := shaderir.Type{Main: shaderir.VectorOf(shaderir.Float, n)}
			_, a := sizeAlign(&col)
			for c := range n {
				walk(&col, base+c*roundUp(4*n, a))
			}
		case t.Main == shaderir.Bool:
			out = append(out, base)
		default:
			n := max(t.VectorElementCount(), 1)
			for i := range n {
				out = append(out, base+4*i)
			}
		}
	}
	walk(t, 0)
	return out
}

func sizeAlign(t *shaderir.Type) (size, align int) {
	switch t.Main {
	case shaderir.Bool:
		return 1, 1
	case shaderir.Int, shaderir.Uint, shaderir.Float:
		return 4, 4
	case shaderir.Vec2, shaderir.IVec2, shaderir.UVec2:
		return 8, 8
	case shaderir.Vec3, shaderir.IVec3, shaderir.UVec3, shaderir.Vec4, shaderir.IVec4, shaderir.UVec4:
		return 16, 16
	case shaderir.Mat2:
		return 16, 8
	case shaderir.Mat3:
		return 48, 16
	case shaderir.Mat4:
		return 64, 16
	case shaderir.Array:
		s, a := sizeAlign(&t.Sub[0])
		return t.Length * roundUp(s, a), a
	}
	panic(fmt.Sprintf("packed: no layout for %s", t))
}

func roundUp(x, a int) int { return (x + a - 1) / a * a }
