package common

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateMarksWhatItDropped(t *testing.T) {
	if got := Truncate("short", 10); got != "short" {
		t.Fatalf("got %q", got)
	}
	if got := Truncate("0123456789abc", 10); got != "0123456789…" {
		t.Fatalf("got %q", got)
	}
}

// The limit counts bytes. A log line or a payload holding an accent or an
// emoji puts a character across the limit, and cutting through one leaves the
// model a replacement glyph instead of the text.
func TestTruncateNeverSplitsACharacter(t *testing.T) {
	// "é" is two bytes, so a character straddles every odd byte index.
	accents := strings.Repeat("é", 40)
	for limit := 9; limit <= 12; limit++ {
		got := Truncate(accents, limit)
		if !utf8.ValidString(got) {
			t.Fatalf("Truncate at %d is not valid UTF-8: %q", limit, got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("Truncate at %d dropped its marker: %q", limit, got)
		}
	}
}

func TestRuneStartStepsBackToACharacter(t *testing.T) {
	text := "a🚀b" // the rocket covers bytes 1 to 4
	for cut, want := range map[int]int{0: 0, 1: 1, 2: 1, 3: 1, 4: 1, 5: 5, 6: 6, 9: 9} {
		if got := RuneStart(text, cut); got != want {
			t.Errorf("RuneStart(%d) = %d, want %d", cut, got, want)
		}
	}
}
