//go:build darwin && !ios

package darwin

import (
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/egoist/mygo/internal/platform"
)

// The checking types of NSTextCheckingResult.
const (
	nsTextCheckingTypeSpelling   = 1 << 1
	nsTextCheckingTypeGrammar    = 1 << 2
	nsTextCheckingTypeCorrection = 1 << 9
)

// spellChecker is the application's shared NSSpellChecker.
func spellChecker() id { return send(class("NSSpellChecker"), "sharedSpellChecker") }

// checking runs fn with AppKit loaded, in its own autorelease pool, as a
// goroutine's thread has none. NSSpellChecker's checking methods are safe
// from any thread (AppKit's 10.6 notes).
func checking(fn func()) {
	load()
	withPool(fn)
}

// CheckText reports the misspelled words of text and, where grammar is
// asked, its grammar issues, in the languages the system's settings name.
func (b *Backend) CheckText(text string, grammar bool) []platform.TextIssue {
	if text == "" {
		return nil
	}
	var out []platform.TextIssue
	checking(func() {
		types := uintptr(nsTextCheckingTypeSpelling)
		if grammar {
			types |= nsTextCheckingTypeGrammar
		}
		offs := utf16Offsets(text)
		units := offs[len(offs)-1].units
		var orthography id
		var words int
		results := send(spellChecker(), "checkString:range:types:options:inSpellDocumentWithTag:orthography:wordCount:",
			uintptr(nsString(text)), 0, uintptr(units), types, 0, 0,
			uintptr(unsafe.Pointer(&orthography)), uintptr(unsafe.Pointer(&words)))
		for _, r := range arrayItems(results) {
			kind := sendInt(r, "resultType")
			rg := objc.Send[nsRange](r, sel("range"))
			switch kind {
			case nsTextCheckingTypeSpelling:
				out = append(out, platform.TextIssue{From: byteAt(offs, rg.Location), To: byteAt(offs, rg.Location+rg.Length)})
			case nsTextCheckingTypeGrammar:
				for _, d := range arrayItems(send(r, "grammarDetails")) {
					v := send(d, "objectForKey:", uintptr(nsString("NSGrammarRange")))
					if v == 0 {
						continue
					}
					dr := objc.Send[nsRange](v, sel("rangeValue"))
					out = append(out, platform.TextIssue{
						From: byteAt(offs, dr.Location), To: byteAt(offs, dr.Location+dr.Length), Grammar: true,
						Description: goString(send(d, "objectForKey:", uintptr(nsString("NSGrammarUserDescription")))),
					})
				}
			}
		}
	})
	return out
}

// Guesses are the checker's corrections for the word from to to of text,
// in bytes, best first.
func (b *Backend) Guesses(text string, from, to int) []string {
	var out []string
	checking(func() {
		offs := utf16Offsets(text)
		lo, hi := unitAt(offs, from), unitAt(offs, to)
		checker := spellChecker()
		guesses := send(checker, "guessesForWordRange:inString:language:inSpellDocumentWithTag:",
			uintptr(lo), uintptr(hi-lo), uintptr(nsString(text)), uintptr(send(checker, "language")), 0)
		for _, g := range arrayItems(guesses) {
			out = append(out, goString(g))
		}
	})
	return out
}

// Correction returns the checker's single automatic correction for the
// word from to to of text, in bytes, or "" when none is offered.
func (b *Backend) Correction(text string, from, to int) string {
	if from < 0 || from >= to || to > len(text) {
		return ""
	}
	var out string
	checking(func() {
		offs := utf16Offsets(text)
		lo, hi := unitAt(offs, from), unitAt(offs, to)
		checker := spellChecker()
		corr := send(checker, "correctionForWordRange:inString:language:inSpellDocumentWithTag:",
			uintptr(lo), uintptr(hi-lo), uintptr(nsString(text)), uintptr(send(checker, "language")), 0)
		if corr != 0 {
			out = goString(corr)
		}
	})
	return out
}

// LearnWord adds word to the user's dictionary, which every app shares.
func (b *Backend) LearnWord(word string) {
	checking(func() { send(spellChecker(), "learnWord:", uintptr(nsString(word))) })
}

// UnlearnWord takes word out of the user's dictionary.
func (b *Backend) UnlearnWord(word string) {
	checking(func() { send(spellChecker(), "unlearnWord:", uintptr(nsString(word))) })
}

// HasLearnedWord reports whether the user's dictionary holds word.
func (b *Backend) HasLearnedWord(word string) bool {
	var ok bool
	checking(func() { ok = sendBool(spellChecker(), "hasLearnedWord:", uintptr(nsString(word))) })
	return ok
}

// CorrectsAutomatically reports whether the system's keyboard settings
// correct spelling automatically.
func (b *Backend) CorrectsAutomatically() bool {
	var ok bool
	checking(func() { ok = sendBool(class("NSSpellChecker"), "isAutomaticSpellingCorrectionEnabled") })
	return ok
}

// PredictsInline reports whether the system's keyboard settings enable
// inline predictive text (macOS 14+).
func (b *Backend) PredictsInline() bool {
	var ok bool
	checking(func() {
		sc := class("NSSpellChecker")
		if sendBool(sc, "respondsToSelector:", uintptr(sel("isAutomaticInlinePredictionEnabled"))) {
			ok = sendBool(sc, "isAutomaticInlinePredictionEnabled")
		}
	})
	return ok
}

type rawCandidate struct {
	loc, length uint
	repl        string
}

type predictReq struct {
	text     string
	caretU16 int
	offs     []offset
	cb       func(string)
}

var (
	predictOnce    sync.Once
	predictBlk     objc.Block
	predictMu      sync.Mutex
	predictPending = map[int]predictReq{}
	predictEarly   = map[int][]rawCandidate{}
)

func initPredictBlock() {
	predictOnce.Do(func() {
		predictBlk = newBlock(func(_ objc.Block, seq int, candidates id) {
			var cands []rawCandidate
			withPool(func() {
				items := arrayItems(candidates)
				cands = make([]rawCandidate, 0, len(items))
				for _, r := range items {
					rg := objc.Send[nsRange](r, sel("range"))
					repl := goString(send(r, "replacementString"))
					cands = append(cands, rawCandidate{loc: rg.Location, length: rg.Length, repl: repl})
				}
			})
			predictMu.Lock()
			req, ok := predictPending[seq]
			if ok {
				delete(predictPending, seq)
			} else {
				predictEarly[seq] = cands
			}
			predictMu.Unlock()
			if ok {
				req.cb(pickPrediction(req, cands))
			}
		})
	})
}

// Predict requests inline text prediction candidates asynchronously at byte
// offset at of text and invokes cb with the predicted suffix, or "".
func (b *Backend) Predict(text string, at int, cb func(string)) {
	if cb == nil {
		return
	}
	if text == "" || at < 0 || at > len(text) {
		cb("")
		return
	}
	if b.running.Load() && !b.IsMainThread() {
		b.post(func() { b.predictNow(text, at, cb) })
		return
	}
	b.predictNow(text, at, cb)
}

func (b *Backend) predictNow(text string, at int, cb func(string)) {
	var (
		early   []rawCandidate
		hasData bool
		req     predictReq
	)
	checking(func() {
		key := appKitString("NSTextCheckingGenerateInlinePredictionsKey")
		if key == 0 {
			early = nil
			hasData = true
			return
		}
		initPredictBlock()
		offs := utf16Offsets(text)
		caretU16 := unitAt(offs, at)
		req = predictReq{text: text, caretU16: caretU16, offs: offs, cb: cb}
		opts := send(class("NSDictionary"), "dictionaryWithObject:forKey:", uintptr(nsBool(true)), uintptr(key))
		seq := sendInt(spellChecker(), "requestCandidatesForSelectedRange:inString:types:options:inSpellDocumentWithTag:completionHandler:",
			uintptr(caretU16), 0, uintptr(nsString(text)), uintptr(nsTextCheckingTypeCorrection), uintptr(opts), 0, uintptr(predictBlk))
		predictMu.Lock()
		if cands, ok := predictEarly[seq]; ok {
			delete(predictEarly, seq)
			early, hasData = cands, true
		} else {
			predictPending[seq] = req
		}
		predictMu.Unlock()
	})
	if hasData {
		if req.cb == nil {
			cb("")
			return
		}
		cb(pickPrediction(req, early))
	}
}

func pickPrediction(req predictReq, cands []rawCandidate) string {
	for _, c := range cands {
		if c.loc+c.length != uint(req.caretU16) {
			continue
		}
		fromByte := byteAt(req.offs, c.loc)
		toByte := byteAt(req.offs, c.loc+c.length)
		if fromByte < 0 || fromByte > toByte || toByte > len(req.text) {
			continue
		}
		prefix := req.text[fromByte:toByte]
		trimmed := strings.TrimRight(c.repl, " ")
		if trimmed == "" || strings.ContainsAny(trimmed, "\r\n") {
			continue
		}
		if !strings.HasPrefix(trimmed, prefix) || len(trimmed) <= len(prefix) {
			continue
		}
		return trimmed[len(prefix):]
	}
	return ""
}

// offset is where a rune of a string starts, in bytes and UTF-16 units.
type offset struct{ bytes, units int }

// utf16Offsets are the starts of text's runes, and its end.
func utf16Offsets(text string) []offset {
	out := make([]offset, 0, len(text)+1)
	units := 0
	for i, r := range text {
		out = append(out, offset{i, units})
		units += utf16.RuneLen(r)
	}
	return append(out, offset{len(text), units})
}

// byteAt is the byte offset of the rune at u UTF-16 units, or of the
// rune holding it.
func byteAt(offs []offset, u uint) int {
	lo, hi := 0, len(offs)-1
	for lo < hi {
		m := (lo + hi + 1) / 2
		if uint(offs[m].units) <= u {
			lo = m
		} else {
			hi = m - 1
		}
	}
	return offs[lo].bytes
}

// unitAt is the UTF-16 offset of the rune at byte b, or of the rune
// holding it.
func unitAt(offs []offset, b int) int {
	lo, hi := 0, len(offs)-1
	for lo < hi {
		m := (lo + hi + 1) / 2
		if offs[m].bytes <= b {
			lo = m
		} else {
			hi = m - 1
		}
	}
	return offs[lo].units
}
