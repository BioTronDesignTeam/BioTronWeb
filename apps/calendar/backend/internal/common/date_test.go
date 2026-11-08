package common

import (
	"testing"
	"time"
)

func TestDateAfterComparesDatesOnly(t *testing.T) {
	day := time.Date(2026, 3, 8, 23, 59, 0, 0, time.UTC)
	cases := []struct {
		name  string
		value time.Time
		want  bool
	}{
		{"same day, later hour", time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC).Add(time.Hour), false},
		{"next day", time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC), true},
		{"day before", time.Date(2026, 3, 7, 23, 59, 0, 0, time.UTC), false},
		{"same date in another location", time.Date(2026, 3, 8, 1, 0, 0, 0, time.FixedZone("EST", -5*3600)), false},
	}
	for _, c := range cases {
		if got := DateAfter(c.value, day); got != c.want {
			t.Errorf("%s: DateAfter = %v, want %v", c.name, got, c.want)
		}
	}
}
