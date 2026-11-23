//go:build !windows

package mygo

import "os"

// readShared reads the file at path, which others may replace meanwhile.
func readShared(path string) ([]byte, error) { return os.ReadFile(path) }

// openShared opens the file at path to read it, which other apps may
// rename or replace meanwhile.
func openShared(path string) (*os.File, error) { return os.Open(path) }

// renameOver renames from over to.
func renameOver(from, to string) error { return os.Rename(from, to) }
