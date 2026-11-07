//go:build !darwin || (!cgo && (!mygo_webgpu || !nofakecgo))

package main

import "fmt"

func main() {
	fmt.Println("layer-membrane needs cgo, or -tags mygo_webgpu,nofakecgo without cgo, on macOS")
}
