//go:build !darwin || (!cgo && (!mygo_webgpu || !nofakecgo))

package main

import "fmt"

func main() {
	fmt.Println("webgpu-layer needs cgo, or -tags mygo_webgpu,nofakecgo without cgo, on macOS")
}
