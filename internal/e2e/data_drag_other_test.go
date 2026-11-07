//go:build !darwin && !(windows && (amd64 || arm64)) && !js

package e2e

import (
	"github.com/caelyreth/evil-mygo"
	"github.com/caelyreth/evil-mygo/transfer"
)

func dropNativeData(*mygo.Window, float64, float64, transfer.Data, transfer.Operation) (transfer.Operation, bool, bool) {
	return transfer.None, false, false
}
