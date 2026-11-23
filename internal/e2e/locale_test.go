//go:build !js

package e2e

import (
	"bytes"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// localeLanguages makes the app written in langs for a test, following
// the user's languages again after it.
func localeLanguages(t *testing.T, langs ...string) {
	t.Helper()
	mygo.App.SetLanguages(langs...)
	t.Cleanup(func() {
		mygo.App.SetLocale("")
		mygo.App.SetLanguages()
	})
}

// TestTextLanguage checks that native UI draws text in the language the
// app shows, which an element's Lang overrides: 直 and 骨 are drawn apart
// in Japanese and in Chinese.
func TestTextLanguage(t *testing.T) {
	if runtime.GOOS == "linux" && (!hasFonts("ja") || !hasFonts("zh-cn")) {
		t.Skip("fontconfig has no Japanese or Chinese fonts (fonts-noto-cjk)")
	}
	localeLanguages(t, "en", "ja", "zh-Hans")
	var seen atomic.Value
	var override atomic.Value
	override.Store("")
	view := func(c *ui.Context) {
		seen.Store(c.Locale())
		ui.Text(c, "直骨").FontSize(48).Lang(override.Load().(string))
	}
	w := newWindow(t, mygo.WindowOptions{Title: "Text language", Width: 200, Height: 100, Content: ui.View(view)})
	capture := func(locale, lang string) []byte {
		t.Helper()
		override.Store(lang)
		mygo.App.SetLocale(locale)
		eventually(t, "the view in "+locale, func() bool { return seen.Load() == locale })
		w.Update(func() {})
		time.Sleep(100 * time.Millisecond)
		data, err := w.CapturePage()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	ja, hans := capture("ja", ""), capture("zh-Hans", "")
	if bytes.Equal(ja, hans) {
		t.Fatal("直骨 looks the same in Japanese and in Chinese")
	}
	// A Japanese quote in a Chinese app, and the other way round.
	if got := capture("zh-Hans", "ja"); !bytes.Equal(got, ja) {
		t.Error("Lang(ja) in a Chinese app draws other glyphs than Japanese")
	}
	if got := capture("ja", "zh-Hans"); !bytes.Equal(got, hans) {
		t.Error("Lang(zh-Hans) in a Japanese app draws other glyphs than Chinese")
	}
}

// hasFonts reports whether fontconfig has fonts for a language.
func hasFonts(lang string) bool {
	out, err := exec.Command("fc-list", ":lang="+lang, "family").Output()
	return err == nil && len(bytes.TrimSpace(out)) > 0
}
