package ui

import (
	"strings"
	"time"
	"weak"

	"github.com/egoist/mygo/transfer"
)

// Services provides persistent window services for custom controls and
// asynchronous work. It owns no frame data and does not retain the window.
// Clipboard and URL methods run on the UI thread; Invalidate may run on
// any goroutine. Models are still updated through Window.Update.
type Services struct{ owner weak.Pointer[engine] }

// Services returns the persistent services of this frame's window.
func (c *Context) Services() Services {
	if c == nil {
		return Services{}
	}
	return c.services
}

func (s Services) Invalidate() {
	if rt := s.owner.Value(); rt != nil {
		rt.host.invalidate()
	}
}

// Now returns the current time of the window's runtime clock (virtual time
// in a Tester), or time.Now() if unattached.
func (s Services) Now() time.Time {
	if rt := s.owner.Value(); rt != nil {
		return rt.now()
	}
	return time.Now()
}
func (s Services) ReadClipboard() string {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		return rt.host.readClipboard()
	}
	return ""
}

func (s Services) WriteClipboard(text string) {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		rt.host.writeClipboard(text)
	}
}

// ReadClipboardData reads the clipboard's representations of formats, as
// a rich text editor reads its own format, HTML and plain text to paste
// the richest it understands; nil formats read all. A clipboard without
// them gives an empty Data.
func (s Services) ReadClipboardData(formats ...transfer.Format) transfer.Data {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		return rt.host.readClipboardData(formats)
	}
	return transfer.Data{}
}

// ClipboardFormats lists the formats on the clipboard without reading
// them, as a paste decides what to read: an image comes as transfer.PNG
// whatever the system's format, which ReadClipboardData converts.
func (s Services) ClipboardFormats() []transfer.Format {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		return rt.host.clipboardFormats()
	}
	return nil
}

// WriteClipboardData puts data on the clipboard, each representation of
// its items in the system's formats, as a rich text editor copies its own
// format, HTML and plain text.
func (s Services) WriteClipboardData(data transfer.Data) {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		rt.host.writeClipboardData(data)
	}
}

// ShowDefinition shows what text means, from the system's dictionaries,
// as macOS's Look Up does: text drawn over where the view drew it, in
// font, from its baseline's start at, in window DIPs, and its meaning
// beside it. It reports whether the window could (CanShowDefinition).
func (s Services) ShowDefinition(text string, at Point, font Font) bool {
	if rt := s.owner.Value(); rt != nil && !rt.closed && text != "" {
		return rt.host.showDefinition(text, at, font)
	}
	return false
}

// CanShowDefinition reports whether the window can look up what text
// means (ShowDefinition): on macOS.
func (s Services) CanShowDefinition() bool {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		return rt.host.canShowDefinition()
	}
	return false
}

func (s Services) OpenURL(url string) {
	if rt := s.owner.Value(); rt != nil && !rt.closed {
		rt.host.openURL(url, nil)
	}
}

// SafeToOpen reports whether a link to url can open in the system when
// others wrote it, as a message, a language model's reply or a pasted
// document: an http, https or mailto address. Other schemes and paths
// open files and run programs (on Windows, "calc" and \\host\x.exe
// run), and javascript: runs script in a browser. Markdown views and the
// rich text editor open only these, unless the app opens links itself.
func SafeToOpen(url string) bool {
	for i := 0; i < len(url); i++ {
		if c := url[i]; c < ' ' || c == 0x7f || c == '\\' {
			return false
		}
	}
	scheme, _, ok := strings.Cut(url, ":")
	if !ok {
		return false
	}
	switch strings.ToLower(scheme) {
	case "http", "https", "mailto":
		return true
	}
	return false
}
