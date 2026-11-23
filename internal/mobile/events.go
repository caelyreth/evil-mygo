package mobile

import (
	"runtime"
	"runtime/debug"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
)

// handle handles an event of the host on the main thread and returns the
// reply the host waits for, if any.
func (b *Backend) handle(msg []byte) []byte {
	r := wire.Reader{Buf: msg}
	op := r.Uvarint()
	var w wire.Writer
	win := func(id int) *window { return b.windows[id] }
	switch op {
	case wire.EventReady:
		b.ready = true
		b.h.Ready()
	case wire.EventActive:
		var e wire.ActiveEvent
		e.Decode(&r)
		if e.Active {
			b.h.DidBecomeActive()
		} else {
			b.h.DidResignActive()
		}
	case wire.EventBackground:
		var e wire.BackgroundEvent
		e.Decode(&r)
		switch {
		case e.Background && !b.background:
			b.background = true
			b.h.PowerEvent("suspend")
		case !e.Background && b.background:
			// Coming back is the app opened again, as clicking its Dock
			// icon is on macOS; launching is not.
			b.background = false
			for _, w := range b.windows {
				if w.s != nil {
					w.s.foreground()
				}
			}
			// Started out of sight, as for a background task or by how iOS
			// says it launches, it's no coming back.
			if b.startedHidden {
				b.startedHidden = false
				break
			}
			b.h.PowerEvent("resume")
			if b.ready {
				b.h.Activated(b.front() != nil)
			}
		}
	case wire.EventThemeChanged:
		var t wire.ThemeReply
		b.call(&wire.Theme{}, &t)
		b.dark = t.Dark
		b.h.ThemeChanged()
		for _, w := range b.windows {
			if w.s != nil {
				w.s.themeChanged()
			}
		}
	case wire.EventLocaleChanged:
		b.forgetInfo()
		b.h.LocaleChanged()
	case wire.EventTimeZoneChanged:
		var e wire.TimeZoneChangedEvent
		e.Decode(&r)
		setTimeZone(e.ID)
		// What shows the time draws it again.
		b.h.LocaleChanged()
	case wire.EventOpenURLs:
		var e wire.OpenURLsEvent
		e.Decode(&r)
		var urls, files []string
		for _, u := range e.URLs {
			if len(u) > 7 && u[:7] == "file://" {
				files = append(files, u[7:])
			} else {
				urls = append(urls, u)
			}
		}
		if len(urls) > 0 {
			b.h.OpenURLs(urls)
		}
		if len(files) > 0 {
			b.h.OpenFiles(files)
		}
	case wire.EventMemoryWarning:
		b.h.MemoryWarning()
		for _, w := range b.windows {
			if w.s != nil {
				w.s.lowMemory()
			}
		}
		runtime.GC()
		debug.FreeOSMemory()
	case wire.EventTerminating:
		b.h.Terminating()

	case wire.EventResized:
		var e wire.ResizedEvent
		e.Decode(&r)
		if w := win(e.ID); w != nil {
			w.resized(&e)
		}
	case wire.EventBackPressed:
		var e wire.BackPressedEvent
		e.Decode(&r)
		reply := wire.BackPressedReply{}
		if w := win(e.ID); w != nil {
			reply.Handled = w.back()
		}
		reply.Encode(&w)
	case wire.EventWindowFront:
		var e wire.WindowFrontEvent
		e.Decode(&r)
		old := b.front()
		var w *window
		if e.ID != 0 {
			w = win(e.ID)
		} else {
			// The app's first scene: the last shown there.
			for _, s := range b.shown {
				if !s.opts.OwnScene {
					w = s
				}
			}
		}
		if w != nil && w.visible {
			b.unshow(w)
			b.shown = append(b.shown, w)
			b.refocus(old)
		}
	case wire.EventPluginEvent:
		var e wire.PluginEventEvent
		e.Decode(&r)
		if b.pluginEvents != nil {
			b.pluginEvents(e.Plugin, e.Name, []byte(e.Data))
		}
	case wire.EventNetworkChanged:
		var e wire.NetworkChangedEvent
		e.Decode(&r)
		b.h.NetworkChanged(platform.NetworkStatus{Online: e.Online, Metered: e.Metered})
	case wire.EventAsyncResult, wire.EventCaptured, wire.EventMessageBoxResult, wire.EventFilesResult, wire.EventNotifyDone,
		wire.EventShareResult, wire.EventMenuPicked, wire.EventPluginResult:
		// Answers to a request: its number first, then the rest.
		rest := r
		req := r.Int()
		b.answer(req, rest)
	case wire.EventNotificationClicked:
		var e wire.NotificationClickedEvent
		e.Decode(&r)
		b.h.NotificationClicked(e.NotificationID)
	default:
		if !b.handleNavigation(op, &r) {
			b.handleSurface(op, &r, &w)
		}
	}
	if !wire.EventWaits(op) {
		return nil
	}
	if w.Buf == nil {
		w.Buf = []byte{}
	}
	return w.Buf
}

// back handles Android's back for the window.
func (w *window) back() bool {
	if w.s != nil {
		return w.s.back()
	}
	return false
}
