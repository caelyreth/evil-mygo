//go:build !unix && !windows

package mygo

import "os"

func lockFile(*os.File) error { return nil }

func lockBusy(error) bool { return false }
