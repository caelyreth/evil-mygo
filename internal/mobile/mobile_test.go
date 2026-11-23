package mobile

import (
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/mobile/wire"
	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

// fakeHost stands for the Swift or Kotlin host: its UI thread is a
// goroutine that runs what is posted to it, in turn.
type fakeHost struct {
	t  *testing.T
	ui chan func()

	mu    sync.Mutex
	calls []uint64
	// answer, when set, answers waited calls by number.
	answer func(op uint64, r *wire.Reader) []byte
	b      *Backend
}

func newFakeHost(t *testing.T) *fakeHost {
	h := &fakeHost{t: t, ui: make(chan func(), 64)}
	go func() {
		for f := range h.ui {
			f()
		}
	}()
	t.Cleanup(func() { close(h.ui) })
	return h
}

func (h *fakeHost) call(msg []byte) []byte {
	r := wire.Reader{Buf: msg}
	op := r.Uvarint()
	h.mu.Lock()
	h.calls = append(h.calls, op)
	answer := h.answer
	h.mu.Unlock()
	if answer != nil {
		if b := answer(op, &r); b != nil {
			return b
		}
	}
	return []byte{}
}

func (h *fakeHost) post() { h.ui <- func() { h.b.t.drain() } }

// onUI runs f on the host's UI thread and waits for it.
func (h *fakeHost) onUI(f func()) {
	done := make(chan struct{})
	h.ui <- func() { f(); close(done) }
	<-done
}

func (h *fakeHost) called(op uint64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.calls {
		if c == op {
			return true
		}
	}
	return false
}

// app is an AppHandler that records what it heard.
type app struct {
	b     *Backend
	ready chan struct{}
	mu    sync.Mutex
	posts []func()
}

func (a *app) Ready() { close(a.ready) }
func (a *app) Dispatch() {
	a.mu.Lock()
	posts := a.posts
	a.posts = nil
	a.mu.Unlock()
	for _, f := range posts {
		f()
	}
}
func (a *app) QuitRequested() bool                               { return true }
func (a *app) Terminating()                                      {}
func (a *app) HotkeyBound(int, string)                           {}
func (a *app) Activated(bool)                                    {}
func (a *app) DidBecomeActive()                                  {}
func (a *app) DidResignActive()                                  {}
func (a *app) OpenURLs([]string)                                 {}
func (a *app) OpenFiles([]string)                                {}
func (a *app) MenuItemClicked(int)                               {}
func (a *app) ThemeChanged()                                     {}
func (a *app) DisplaysChanged()                                  {}
func (a *app) LocaleChanged()                                    {}
func (a *app) PowerEvent(string)                                 {}
func (a *app) NetworkChanged(platform.NetworkStatus)             {}
func (a *app) HotkeyPressed(int)                                 {}
func (a *app) NotificationClicked(string)                        {}
func (a *app) MemoryWarning()                                    {}

// startBackend runs a backend with a fake host, its loop on a goroutine of its
// own, the main thread, and returns them once the app is ready.
func startBackend(t *testing.T) (*Backend, *fakeHost) {
	h := newFakeHost(t)
	theHost = h
	t.Cleanup(func() { theHost = nil })
	b := New()
	h.b = b
	a := &app{b: b, ready: make(chan struct{})}
	inited := make(chan error, 1)
	go func() {
		if err := b.Init(a, platform.AppOptions{}); err != nil {
			inited <- err
			return
		}
		inited <- nil
		b.Run()
	}()
	if err := <-inited; err != nil {
		t.Fatal(err)
	}
	wait.For(t, "the app to be ready", a.ready)
	t.Cleanup(b.Quit)
	return b, h
}

// onMain runs f on the backend's main thread and waits for it.
func onMain(b *Backend, f func()) {
	a := b.h.(*app)
	done := make(chan struct{})
	a.mu.Lock()
	a.posts = append(a.posts, func() { f(); close(done) })
	a.mu.Unlock()
	b.Signal()
	<-done
}

// winHandler hears of a window; what a test does not set panics.
type winHandler struct {
	platform.WindowHandler
}

func encodeEvent(f func(w *wire.Writer)) []byte {
	var w wire.Writer
	f(&w)
	return w.Buf
}

// A message box answers once the host tells which button was chosen.
func TestMessageBox(t *testing.T) {
	b, h := startBackend(t)
	got := make(chan int, 1)
	onMain(b, func() {
		b.Dialogs().ShowMessageBox(nil, &platform.MessageBoxOptions{Message: "Sure?", Buttons: []string{"Yes", "No"}},
			func(r platform.MessageBoxResult, err error) { got <- r.Response })
	})
	wait.Until(t, "the message box", func() bool { return h.called(wire.CallMessageBox) })
	h.onUI(func() {
		deliver(encodeEvent(func(w *wire.Writer) {
			w.Uvarint(wire.EventMessageBoxResult)
			w.Int(1) // the request
			w.Int(1) // No
		}), true)
	})
	if r := wait.For(t, "the answer", got); r != 1 {
		t.Errorf("Response = %d, want 1", r)
	}
}
