//go:build darwin && !((mygo_webgpu && nofakecgo) || cgo)

package main

import "fmt"

func main() {
	fmt.Println("webgpu-canvas needs normal cgo, or -tags mygo_webgpu,nofakecgo without cgo")
}
