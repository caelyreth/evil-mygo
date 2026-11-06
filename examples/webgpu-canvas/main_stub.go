//go:build !darwin

package main

import "fmt"

func main() {
	fmt.Println("webgpu-canvas is available on macOS through go-webgpu")
}
