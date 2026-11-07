//go:build js

package web

import "github.com/caelyreth/evil-mygo/internal/platform"

func (app) AddRecentDocument(string) error      { return errUnsupported }
func (app) ClearRecentDocuments() error         { return errUnsupported }
func (app) SetJumpList(platform.JumpList) error { return errUnsupported }
