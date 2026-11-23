// Package mygo is a desktop application framework written in pure Go,
// without cgo. Windows render native UI that MyGo draws itself, written in
// Go with package [github.com/egoist/mygo/ui]:
//
//	mygo.NewWindow(mygo.WindowOptions{Title: "Counter", Content: ui.View(app.view)})
//
// The rest of the package covers the desktop: App (lifecycle, deep links,
// file associations, the Dock), Window, NewMenu and NewTray, Dialog,
// NewNotification, Clipboard, Shell, Screen, Theme, Power, GlobalShortcut,
// and Updater.
//
// # Threading
//
// The system's UI toolkits (AppKit, GTK, Win32) must be driven from the
// process' main thread. MyGo locks the main goroutine to the main thread
// during package initialization, so App.Run must be called from main().
// Every other function and method in this package is safe to call from any
// goroutine: calls made off the main thread are forwarded to it and wait for
// the result.
//
// Before App.Run, main sets the app up: it binds services, adds listeners
// and makes settings, such as Theme.SetSource, which apply once the app
// starts. Calls that need the running app, such as the clipboard, displays
// or dialogs, panic when main makes them before Run; other goroutines wait
// for the app to start.
//
// Event listeners (OnClose, OnFocus, ...) run on the main thread; keep them
// short and move slow work to a goroutine. Methods of bound services run on
// goroutines of their own, one per call, so they may block.
package mygo

import (
	"runtime"
	"sync"

	"github.com/egoist/mygo/internal/platform"
)

// Version is the MyGo version.
const Version = "0.10.0"

func init() {
	// Cocoa, GTK and Win32 all require the UI to live on the thread that
	// started the process. Keep the main goroutine there.
	runtime.LockOSThread()
}

var (
	backendOnce sync.Once
	theBackend  platform.Backend
)

// backend returns the platform backend, creating it on first use.
func backend() platform.Backend {
	backendOnce.Do(func() { theBackend = newBackend() })
	return theBackend
}
