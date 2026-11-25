package mygo

import "github.com/egoist/mygo/internal/platform"

// SpellingModule checks spelling and grammar with the system's checker.
// Use the Spelling singleton.
type SpellingModule struct{}

// Spelling checks spelling and grammar with the system's checker, in the
// languages the system's settings name: NSSpellChecker on macOS. Elsewhere
// Available is false and the methods find nothing.
//
//	for _, is := range mygo.Spelling.Check(text, true) {
//		underline(text[is.From:is.To], is.Grammar)
//	}
//
// The methods are safe from any goroutine and work before Run. A check
// takes about as long as the system takes over the text, so an app checks
// what changed, not the whole document each time.
var Spelling SpellingModule

// TextIssue is a misspelled word, or a grammar issue: bytes From to To of
// the text checked. Description says what is wrong, for grammar.
type TextIssue = platform.TextIssue

func spelling() (platform.SpellingBackend, bool) {
	s, ok := backend().(platform.SpellingBackend)
	return s, ok
}

// Available reports whether there is a system checker.
func (SpellingModule) Available() bool {
	_, ok := spelling()
	return ok
}

// Check reports text's misspelled words and, with grammar, its grammar
// issues.
func (SpellingModule) Check(text string, grammar bool) []TextIssue {
	if s, ok := spelling(); ok {
		return s.CheckText(text, grammar)
	}
	return nil
}

// Guesses are the corrections for the word from to to of text, in bytes,
// best first.
func (SpellingModule) Guesses(text string, from, to int) []string {
	if s, ok := spelling(); ok && 0 <= from && from <= to && to <= len(text) {
		return s.Guesses(text, from, to)
	}
	return nil
}

// Correction returns a single automatic correction for the word from to to
// of text, in bytes, or "" when the checker offers none.
func (SpellingModule) Correction(text string, from, to int) string {
	if s, ok := spelling(); ok && 0 <= from && from < to && to <= len(text) {
		return s.Correction(text, from, to)
	}
	return ""
}

// Learn adds word to the user's dictionary, which every app shares, so it
// is no longer misspelled.
func (SpellingModule) Learn(word string) {
	if s, ok := spelling(); ok {
		s.LearnWord(word)
	}
}

// Unlearn takes word out of the user's dictionary.
func (SpellingModule) Unlearn(word string) {
	if s, ok := spelling(); ok {
		s.UnlearnWord(word)
	}
}

// Learned reports whether the user's dictionary holds word.
func (SpellingModule) Learned(word string) bool {
	s, ok := spelling()
	return ok && s.HasLearnedWord(word)
}

// CorrectsAutomatically reports whether the system's settings correct
// spelling as one types (on macOS, Keyboard › Text Input › Correct
// spelling automatically), for an app that offers to follow them.
func (SpellingModule) CorrectsAutomatically() bool {
	s, ok := spelling()
	return ok && s.CorrectsAutomatically()
}
