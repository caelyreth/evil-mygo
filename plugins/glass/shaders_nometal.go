//go:build !(darwin && !ios)

package glass

// The Metal libraries compiled ahead of time are for macOS alone.
var metalLibrary, blurMetalLibrary, progMetalLibrary, edgeMetalLibrary []byte

const metalSum, blurMetalSum, progMetalSum, edgeMetalSum = "", "", "", ""
