//go:build darwin && !ios

package darwin

import (
	"os"
	"slices"
	"syscall"

	"github.com/egoist/mygo/i18n"
)

// AppKit says its own texts, the items it adds to an app's menus itself
// (AutoFill, Start Dictation, Emoji & Symbols, the window's arrangements),
// its panels and its alerts, in the language Core Foundation works out as
// the process starts: from the launch arguments, then the app's defaults,
// then the user's. MYGO_LOCALE (`mygo dev -lang`) chooses the app's
// language for one run without saving it, so neither defaults say it; the
// launch arguments are the one way to tell Core Foundation, which reads
// them from the process's own argv, never from what the process sets
// later. So a process started with MYGO_LOCALE and without them starts
// itself again at once, before anything of the app runs, with
// -AppleLanguages added, as one would launch it in a language by hand.
// Started so, it takes the arguments out of os.Args again, so the app's
// own flags never see them.

// launchLanguageArg is the argument naming the languages AppKit reads.
const launchLanguageArg = "-AppleLanguages"

func init() {
	tag := i18n.Normalize(os.Getenv("MYGO_LOCALE"))
	if at := slices.Index(os.Args, launchLanguageArg); at > 0 {
		if at+1 < len(os.Args) {
			os.Args = slices.Delete(os.Args, at, at+2)
		}
		return
	}
	if tag == "" {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	args := append(slices.Clone(os.Args), launchLanguageArg, `("`+tag+`")`)
	// Exec only returns when it failed; the app then runs as it is, its
	// own words in its language and AppKit's in the user's.
	_ = syscall.Exec(exe, args, os.Environ())
}
