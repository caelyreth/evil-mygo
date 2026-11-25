//go:build !windows

package glass

// The Direct3D bytecode compiled ahead of time is for Windows alone.
var hlslBytecode, blurBytecode, progBytecode, edgeBytecode []byte

const hlslSum, blurHLSLSum, progHLSLSum, edgeHLSLSum = "", "", "", ""
