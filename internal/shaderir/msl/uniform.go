package msl

import (
	"github.com/caelyreth/evil-mygo/internal/shaderir"
	"github.com/caelyreth/evil-mygo/internal/shaderir/packed"
)

// UniformLayout returns where each of p's uniform variables lies in the
// packed uniform buffer, in bytes, and the buffer's size.
func UniformLayout(p *shaderir.Program) (offsets []int, size int) {
	return packed.UniformLayout(p)
}

// UniformLanes returns, for each lane of p's uniforms as package interp
// keeps them, its offset in the packed uniform buffer, and the buffer's
// size.
func UniformLanes(p *shaderir.Program) (lanes []int, size int) {
	return packed.UniformLanes(p)
}

// TypeLanes returns the byte offsets of a value's lanes in the packed
// uniform layout.
func TypeLanes(t *shaderir.Type) []int {
	return packed.TypeLanes(t)
}
