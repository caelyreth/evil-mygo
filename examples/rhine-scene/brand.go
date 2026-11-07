//go:build darwin && ((mygo_webgpu && nofakecgo) || cgo)

package main

import (
	_ "embed"

	"github.com/caelyreth/evil-mygo/ui"
)

//go:embed assets/rhine-lab.svg
var rhineLabSVG []byte

var rhineLabLogo = ui.MustParseSVG(rhineLabSVG)
