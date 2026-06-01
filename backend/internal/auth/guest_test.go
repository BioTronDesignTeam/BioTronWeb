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
	}
	for _, tc := range cases {
		if got := guestSessionExpiry(tc.now); !got.Equal(tc.want) {
			t.Errorf("%s: guestSessionExpiry = %s, want %s", tc.name, got.UTC(), tc.want.UTC())
		}
	}
}
