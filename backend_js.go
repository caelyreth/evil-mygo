//go:build js

package mygo

import (
	"github.com/caelyreth/evil-mygo/internal/platform"
	"github.com/caelyreth/evil-mygo/internal/web"
)

func newBackend() platform.Backend { return web.New() }
