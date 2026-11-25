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
