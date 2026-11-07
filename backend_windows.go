//go:build windows && (amd64 || arm64)

package mygo

import (
	"github.com/caelyreth/evil-mygo/internal/platform"
	win "github.com/caelyreth/evil-mygo/internal/windows"
)

func newBackend() platform.Backend { return win.New() }

func linkContentRules() { win.LinkContentRules() }

func linkNowPlaying() { win.LinkNowPlaying() }

func linkNotificationImages() { win.LinkNotificationImages() }

func linkMenuIcons() { win.LinkMenuIcons() }

func linkContextMenus() { win.LinkContextMenus() }
