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
			name: "summer EDT",
			now:  time.Date(2026, 6, 1, 18, 30, 0, 0, time.UTC),
			want: time.Date(2026, 6, 2, 4, 0, 0, 0, time.UTC),
		},
		{
			name: "winter EST",
			now:  time.Date(2026, 1, 15, 18, 30, 0, 0, time.UTC),
			want: time.Date(2026, 1, 16, 5, 0, 0, 0, time.UTC),
		},
		{
			name: "spring-forward eve",
			now:  time.Date(2026, 3, 8, 1, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC),
		},
		{
			name: "spring-forward day",
			now:  time.Date(2026, 3, 8, 16, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC),
		},
		{
			name: "fall-back eve",
			now:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 11, 1, 4, 0, 0, 0, time.UTC),
		},
		{
			name: "fall-back day",
			now:  time.Date(2026, 11, 1, 17, 0, 0, 0, time.UTC),
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
		{"ABCDEFGHIJKL", "ABCDEFGHIJKL"},
		{"abcdefghijkl", "ABCDEFGHIJKL"},
		{"ABCD-EFGH-IJKL", "ABCDEFGHIJKL"},
		{"abcd efgh ijkl", "ABCDEFGHIJKL"},
		{" a1b2-c3 ", "A1B2C3"},
		{"AB!@#cd", "ABCD"},
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
	if got := normalizeGuestKey(formatGuestKey("ABCDEFGHIJKL")); got != "ABCDEFGHIJKL" {
		t.Errorf("round-trip = %q, want ABCDEFGHIJKL", got)
	}
}
