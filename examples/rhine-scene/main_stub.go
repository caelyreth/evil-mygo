//go:build !darwin || (!cgo && (!mygo_webgpu || !nofakecgo))

package main

import "fmt"

func main() {
	fmt.Println("rhine-scene needs macOS WebGPU: CGO_ENABLED=0 go run -tags mygo_webgpu,nofakecgo ./examples/rhine-scene")
}
