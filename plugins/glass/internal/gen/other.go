//go:build !windows && !js

package main

import "errors"

func compile(string) (compiled string, code []byte, err error) {
	return "", nil, errors.New("the glass's Direct3D shaders compile on Windows only")
}
