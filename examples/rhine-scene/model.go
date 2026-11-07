package main

import "math"

type vec3 struct{ x, y, z float32 }

func add(a, b vec3) vec3         { return vec3{a.x + b.x, a.y + b.y, a.z + b.z} }
func sub(a, b vec3) vec3         { return vec3{a.x - b.x, a.y - b.y, a.z - b.z} }
func mul(a vec3, s float32) vec3 { return vec3{a.x * s, a.y * s, a.z * s} }
func dot(a, b vec3) float32      { return a.x*b.x + a.y*b.y + a.z*b.z }
func cross(a, b vec3) vec3 {
	return vec3{a.y*b.z - a.z*b.y, a.z*b.x - a.x*b.z, a.x*b.y - a.y*b.x}
}
func unit(a vec3) vec3 {
	length := float32(math.Sqrt(float64(dot(a, a))))
	if length == 0 {
		return vec3{}
	}
	return mul(a, 1/length)
}

type matrix [16]float32

func product(a, b matrix) (out matrix) {
	for c := range 4 {
		for r := range 4 {
			for k := range 4 {
				out[c*4+r] += a[k*4+r] * b[c*4+k]
			}
		}
	}
	return
}
func point(m matrix, v vec3) vec3 {
	return vec3{m[0]*v.x + m[4]*v.y + m[8]*v.z + m[12], m[1]*v.x + m[5]*v.y + m[9]*v.z + m[13], m[2]*v.x + m[6]*v.y + m[10]*v.z + m[14]}
}

type vertex struct {
	position [4]float32
	normal   [4]float32
	uv       [4]float32
}
type material struct {
	color  [4]float32 // Linear RGB and roughness.
	optics [4]float32 // Metalness, transmission, thickness, surface category.
}
type mesh struct {
	name         string
	first, count uint32
	material     int
	array        bool
	glass        bool
}
type model struct {
	vertices  []vertex
	indices   []uint32
	materials []material
	meshes    []mesh
}
