package auth

import (
	"testing"
	"time"
)

func TestGuestSessionExpiry(t *testing.T) {
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			// Summer = EDT (UTC-4): 18:30 UTC = 14:30 EDT → next midnight 00:00 EDT = 04:00 UTC.
			name: "summer EDT",
			now:  time.Date(2026, 6, 1, 18, 30, 0, 0, time.UTC),
			want: time.Date(2026, 6, 2, 4, 0, 0, 0, time.UTC),
		},
		{
			// Winter = EST (UTC-5): 18:30 UTC = 13:30 EST → next midnight 00:00 EST = 05:00 UTC.
			name: "winter EST",
			now:  time.Date(2026, 1, 15, 18, 30, 0, 0, time.UTC),
			want: time.Date(2026, 1, 16, 5, 0, 0, 0, time.UTC),
		},
		{
			// Eve of spring-forward (2026-03-08): evening of 03-07 is still EST → next
			// Eastern midnight 03-08 00:00 EST = 05:00 UTC (the jump is later, at 02:00).
			name: "spring-forward eve",
			now:  time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC), // 2026-03-07 20:00 EST
			want: time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC),
		},
		{
			// Afternoon of spring-forward day (now EDT, UTC-4) → next midnight
			// 2026-03-09 00:00 EDT = 04:00 UTC.
			name: "spring-forward day",
			now:  time.Date(2026, 3, 8, 16, 0, 0, 0, time.UTC), // 2026-03-08 12:00 EDT
			want: time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC),
		},
		{
			// Eve of fall-back (2026-11-01): evening of 10-31 is still EDT → next
			// Eastern midnight 11-01 00:00 EDT = 04:00 UTC (the fall-back is at 02:00).
			name: "fall-back eve",
			now:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), // 2026-10-31 20:00 EDT
			want: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC),
		},
		{
			// Afternoon of fall-back day (now EST, UTC-5) → next midnight
			// 2026-11-02 00:00 EST = 05:00 UTC.
			name: "fall-back day",
			now:  time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC), // 2026-11-01 12:00 EST
			want: time.Date(2026, 11, 2, 5, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		if got := guestSessionExpiry(tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: guestSessionExpiry = %s, want %s", tc.name, got.UTC(), tc.want.UTC())
		}
	}
}

func TestNormalizeGuestKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ABCDEFGHIJKL", "ABCDEFGHIJKL"},   // already canonical
		{"abcdefghijkl", "ABCDEFGHIJKL"},   // lowercase → upper
		{"ABCD-EFGH-IJKL", "ABCDEFGHIJKL"}, // dashes stripped (the display format)
		{"abcd efgh ijkl", "ABCDEFGHIJKL"}, // spaces stripped
		{" a1b2-c3 ", "A1B2C3"},            // trim + upper + dashes
		{"AB!@#cd", "ABCD"},                // stray symbols dropped
	}
	for _, tc := range cases {
		if got := normalizeGuestKey(tc.in); got != tc.want {
			t.Errorf("normalizeGuestKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatGuestKey(t *testing.T) {
	if got := formatGuestKey("ABCDEFGHIJKL"); got != "ABCD-EFGH-IJKL" {
		t.Errorf("formatGuestKey = %q, want ABCD-EFGH-IJKL", got)
	}
	// A normalized display key round-trips back to canonical.
	if got := normalizeGuestKey(formatGuestKey("ABCDEFGHIJKL")); got != "ABCDEFGHIJKL" {
		t.Errorf("round-trip = %q, want ABCDEFGHIJKL", got)
	}
}
