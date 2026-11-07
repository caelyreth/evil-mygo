//go:build !darwin && !(linux && (amd64 || arm64)) && !(windows && (amd64 || arm64)) && !js

package mygo

import (
	"github.com/caelyreth/evil-mygo/internal/platform"
	"github.com/caelyreth/evil-mygo/internal/unsupported"
)

func newBackend() platform.Backend { return unsupported.New() }
