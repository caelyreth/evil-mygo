//go:build darwin && !ios

package darwin

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestTheSystemCheckerFindsAMisspelledWordInBytes(t *testing.T) {
	b := &Backend{}
	text := "Café: I wnet home."
	issues := b.CheckText(text, false)
	i := slices.IndexFunc(issues, func(is platform.TextIssue) bool { return text[is.From:is.To] == "wnet" })
	if i < 0 {
		t.Fatalf("wnet not found: %+v", issues)
	}
	if g := b.Guesses(text, issues[i].From, issues[i].To); !slices.Contains(g, "went") {
		t.Fatalf("guesses %q", g)
	}
}

func TestTheSystemCheckerOffersACorrectionForAMisspelledWord(t *testing.T) {
	b := &Backend{}
	if got := b.Correction("teh ", 0, 3); got != "the" {
		t.Fatalf("Correction(\"teh \", 0, 3) = %q, want \"the\"", got)
	}
	if got := b.Correction("the ", 0, 3); got != "" {
		t.Fatalf("Correction(\"the \", 0, 3) = %q, want \"\"", got)
	}
	if got := b.Correction("", 0, 0); got != "" {
		t.Fatalf("Correction(\"\", 0, 0) = %q, want \"\"", got)
	}
	if got := b.Correction("teh", 2, 1); got != "" {
		t.Fatalf("Correction(\"teh\", 2, 1) = %q, want \"\"", got)
	}
	if got := b.Correction("teh", 0, 10); got != "" {
		t.Fatalf("Correction(\"teh\", 0, 10) = %q, want \"\"", got)
	}
}

func TestPickPredictionExtractsSuffixAndSkipsNonPrefix(t *testing.T) {
	wordText := "it is uncomp"
	reqWord := predictReq{
		text:     wordText,
		caretU16: len(wordText),
		offs:     utf16Offsets(wordText),
	}
	if got := pickPrediction(reqWord, []rawCandidate{
		{loc: 6, length: 6, repl: "uncomp "},
		{loc: 6, length: 6, repl: "uncompromising "},
	}); got != "romising" {
		t.Fatalf("mid-word pickPrediction = %q, want \"romising\"", got)
	}
	if got := pickPrediction(reqWord, []rawCandidate{
		{loc: 6, length: 6, repl: "different "},
	}); got != "" {
		t.Fatalf("non-prefix pickPrediction = %q, want \"\"", got)
	}

	nextText := "I am looking forward to "
	reqNext := predictReq{
		text:     nextText,
		caretU16: len(nextText),
		offs:     utf16Offsets(nextText),
	}
	if got := pickPrediction(reqNext, []rawCandidate{
		{loc: uint(len(nextText)), length: 0, repl: "seeing "},
	}); got != "seeing" {
		t.Fatalf("next-word pickPrediction = %q, want \"seeing\"", got)
	}
}

func TestTheSystemCheckerPredictsInlineCompletions(t *testing.T) {
	b := &Backend{}
	predict := func(text string) string {
		ch := make(chan string, 1)
		b.Predict(text, len(text), func(s string) { ch <- s })
		select {
		case got := <-ch:
			return got
		case <-time.After(3 * time.Second):
			t.Fatalf("Predict(%q) timed out", text)
			return ""
		}
	}
	// Empty / invalid offsets resolve immediately with "".
	if got := predict(""); got != "" {
		t.Fatalf("Predict(\"\") = %q, want \"\"", got)
	}
	got := predict("it is uncomp")
	if b.PredictsInline() && got == "" {
		t.Fatalf("Predict(\"it is uncomp\") = \"\", want a completion when PredictsInline is true")
	}
	if got != "" && !strings.HasPrefix("uncompromising", "uncomp"+got) && len(got) == 0 {
		t.Fatalf("unexpected completion %q", got)
	}
	next := predict("I am looking forward to ")
	if b.PredictsInline() && next == "" {
		t.Fatalf("Predict(\"I am looking forward to \") = \"\", want next-word prediction")
	}
}
