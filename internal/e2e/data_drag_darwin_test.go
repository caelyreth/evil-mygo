//go:build darwin

package e2e

import (
	"github.com/caelyreth/evil-mygo"
	"github.com/caelyreth/evil-mygo/internal/darwin"
	"github.com/caelyreth/evil-mygo/transfer"
)

func dropNativeData(w *mygo.Window, x, y float64, d transfer.Data, ops transfer.Operation) (operation transfer.Operation, dropped, supported bool) {
	mygo.RunOnMain(func() { operation, dropped = darwin.TestDropData(w.NativeHandle(), x, y, d, ops) })
	return operation, dropped, true
}
