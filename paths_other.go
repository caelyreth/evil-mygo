//go:build !windows

package mygo

// knownFolder is "" where the system keeps no known folders.
func knownFolder(PathName) string { return "" }
