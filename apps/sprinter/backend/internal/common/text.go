package common

import "unicode/utf8"

// RuneStart moves a byte index back to the start of the character it falls
// in. Every limit in this backend counts bytes, and a text holding an accent
// or an emoji puts a character across the limit; cutting mid-character
// leaves the model a replacement glyph, and Discord a message that is not
// valid UTF-8. An index at or past the end of text is returned as it is.
func RuneStart(text string, cut int) int {
	for cut > 0 && cut < len(text) && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return cut
}

// Truncate keeps at most limit bytes of text, cut at the start of a
// character, and marks a cut with "…". It shortens one field, such as a
// event field or an error body: useful at a glance, unbounded in principle.
func Truncate(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:RuneStart(text, limit)] + "…"
}
