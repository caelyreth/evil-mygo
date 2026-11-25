//go:build darwin && !ios

package darwin

import (
	"slices"
	"testing"

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
