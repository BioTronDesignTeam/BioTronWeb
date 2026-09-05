package uptime

import (
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

const heartbeat = 5 * time.Minute
const maxGap = 3 * heartbeat

var windowStart = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

func at(offset time.Duration, ok bool) model.HealthPoint {
	return model.HealthPoint{OK: ok, CheckedAt: windowStart.Add(offset)}
}

// heartbeats fills [from, to) with an ok/not-ok heartbeat every interval so a
// window looks continuously observed.
func heartbeats(from, to time.Duration, ok bool) []model.HealthPoint {
	points := make([]model.HealthPoint, 0)
	for offset := from; offset < to; offset += heartbeat {
		points = append(points, at(offset, ok))
	}
	return points
}

func dayWindow() Window {
	return Window{Start: windowStart, End: windowStart.Add(24 * time.Hour)}
}

func percent(t *testing.T, result Result) float64 {
	t.Helper()
	value := result.Percent()
	if value == nil {
		t.Fatalf("expected a percentage, got nil (result = %#v)", result)
	}
	return *value
}

func TestFullyUpWindowIsHundredPercent(t *testing.T) {
	samples := append([]model.HealthPoint{at(-heartbeat, true)}, heartbeats(0, 24*time.Hour, true)...)
	result := Compute(samples, dayWindow(), maxGap)

	if got := percent(t, result); got != 100 {
		t.Fatalf("uptime = %v, want 100", got)
	}
	if result.Down != 0 || result.Unknown != 0 {
		t.Fatalf("expected no downtime or unknown time: %#v", result)
	}
	if result.Up != 24*time.Hour {
		t.Fatalf("up = %s, want 24h", result.Up)
	}
}

func TestOneDownSegmentIsWeightedByDuration(t *testing.T) {
	// Down for the third and fourth hours: two hours out of twenty-four.
	samples := heartbeats(0, 2*time.Hour, true)
	samples = append(samples, heartbeats(2*time.Hour, 4*time.Hour, false)...)
	samples = append(samples, heartbeats(4*time.Hour, 24*time.Hour, true)...)

	result := Compute(samples, dayWindow(), maxGap)
	if result.Down != 2*time.Hour {
		t.Fatalf("down = %s, want 2h", result.Down)
	}
	if result.Up != 22*time.Hour {
		t.Fatalf("up = %s, want 22h", result.Up)
	}
	if got := percent(t, result); got != 91.67 {
		t.Fatalf("uptime = %v, want 91.67", got)
	}
}

func TestRowCountingWouldOverWeightFlapping(t *testing.T) {
	// Twenty-three quiet hours of heartbeats, then a final hour flapping every
	// two seconds. Transitions cluster densely, so count(ok)/count(*) would
	// call this day about 57% up. Half of one hour was actually lost, so the
	// honest figure is 97.92%.
	samples := heartbeats(0, 23*time.Hour, true)
	okRows, totalRows := len(samples), len(samples)
	for offset, ok := 23*time.Hour, false; offset < 24*time.Hour; offset, ok = offset+2*time.Second, !ok {
		samples = append(samples, at(offset, ok))
		totalRows++
		if ok {
			okRows++
		}
	}

	naive := float64(okRows) / float64(totalRows) * 100
	if naive > 60 {
		t.Fatalf("fixture is not adversarial enough: naive row ratio = %v", naive)
	}
	result := Compute(samples, dayWindow(), maxGap)
	if result.Down != 30*time.Minute {
		t.Fatalf("down = %s, want 30m", result.Down)
	}
	if got := percent(t, result); got != 97.92 {
		t.Fatalf("uptime = %v, want 97.92 (row counting would say %.2f)", got, naive)
	}
}

func TestStateCarriedInFromBeforeTheWindowIsClipped(t *testing.T) {
	// The only row is six hours before the window opens and says "down", with
	// heartbeats continuing through the window so nothing looks unobserved.
	samples := heartbeats(-6*time.Hour, 3*time.Hour, false)
	samples = append(samples, heartbeats(3*time.Hour, 24*time.Hour, true)...)

	result := Compute(samples, dayWindow(), maxGap)
	if result.Down != 3*time.Hour {
		t.Fatalf("down = %s, want the clipped 3h inside the window, not 9h", result.Down)
	}
	if result.Up != 21*time.Hour {
		t.Fatalf("up = %s, want 21h", result.Up)
	}
	if result.Unknown != 0 {
		t.Fatalf("unknown = %s, want none", result.Unknown)
	}
}

func TestLongGapIsUnknownAndLeavesTheDenominator(t *testing.T) {
	// Logger itself was down from hour two to hour six, so nothing was
	// observed. The four missing hours must not be counted as up.
	samples := heartbeats(0, 2*time.Hour, true)
	samples = append(samples, heartbeats(6*time.Hour, 24*time.Hour, true)...)

	result := Compute(samples, dayWindow(), maxGap)
	// The gap runs from the final pre-gap heartbeat at 01:55 to 06:00.
	if result.Unknown != 4*time.Hour+heartbeat {
		t.Fatalf("unknown = %s, want 4h05m of unobserved time", result.Unknown)
	}
	if result.Down != 0 {
		t.Fatalf("down = %s, want none: a gap is not an outage", result.Down)
	}
	if result.Up+result.Down+result.Unknown != 24*time.Hour {
		t.Fatalf("segments do not tile the window: %#v", result)
	}
	if got := percent(t, result); got != 100 {
		t.Fatalf("uptime = %v, want 100 over the observed time only", got)
	}
	ratio := result.UnknownRatio(24 * time.Hour)
	if ratio < 0.16 || ratio > 0.18 {
		t.Fatalf("unknown ratio = %v, want roughly 4h/24h", ratio)
	}
}

func TestGapShorterThanThreeHeartbeatsStillCounts(t *testing.T) {
	// A ten-minute gap is two missed heartbeats, which happens on a slow poll
	// and must not be thrown away as unknown.
	samples := []model.HealthPoint{at(0, true), at(10*time.Minute, true)}
	samples = append(samples, heartbeats(15*time.Minute, 24*time.Hour, true)...)

	result := Compute(samples, dayWindow(), maxGap)
	if result.Unknown != 0 {
		t.Fatalf("unknown = %s, want none for a gap within tolerance", result.Unknown)
	}
}

func TestWindowWithNoRowsIsNullNotHundred(t *testing.T) {
	result := Compute(nil, dayWindow(), maxGap)
	if result.Percent() != nil {
		t.Fatalf("uptime = %v, want nil for a window with no data", *result.Percent())
	}
	if result.Unknown != 24*time.Hour {
		t.Fatalf("unknown = %s, want the whole window", result.Unknown)
	}
	if result.UnknownRatio(24*time.Hour) != 1 {
		t.Fatalf("unknown ratio = %v, want 1", result.UnknownRatio(24*time.Hour))
	}
}

func TestWindowWhoseRowsAllPostDateItIsNull(t *testing.T) {
	// A component added to the catalog today has no history for a window three
	// months ago.
	samples := heartbeats(48*time.Hour, 50*time.Hour, true)
	result := Compute(samples, dayWindow(), maxGap)
	if result.Percent() != nil {
		t.Fatalf("uptime = %v, want nil", *result.Percent())
	}
}

func TestAlmostPerfectWindowIsClampedBelowHundred(t *testing.T) {
	// One second of downtime in a day rounds to 100.00 but must not be shown as
	// a perfect window.
	samples := heartbeats(0, 12*time.Hour, true)
	samples = append(samples,
		at(12*time.Hour, false),
		at(12*time.Hour+time.Second, true),
	)
	samples = append(samples, heartbeats(12*time.Hour+heartbeat, 24*time.Hour, true)...)

	result := Compute(samples, dayWindow(), maxGap)
	if result.Down != time.Second {
		t.Fatalf("down = %s, want 1s", result.Down)
	}
	if got := percent(t, result); got != 99.99 {
		t.Fatalf("uptime = %v, want the 99.99 clamp", got)
	}
}

func TestFullyDownWindowIsZero(t *testing.T) {
	samples := heartbeats(-heartbeat, 24*time.Hour, false)
	result := Compute(samples, dayWindow(), maxGap)
	if got := percent(t, result); got != 0 {
		t.Fatalf("uptime = %v, want 0", got)
	}
}

func TestBucketedSplitsOneGapConsistentlyAcrossDayBoundaries(t *testing.T) {
	// A twenty-minute observation gap straddling midnight is one gap. Judging
	// each day's clipped fragment on its own would call the five-minute side
	// observed and the fifteen-minute side unknown, which is incoherent.
	dayOne := Window{Start: windowStart, End: windowStart.Add(24 * time.Hour)}
	dayTwo := Window{Start: dayOne.End, End: dayOne.End.Add(24 * time.Hour)}
	samples := heartbeats(0, 24*time.Hour-4*time.Minute, true)
	samples = append(samples, at(24*time.Hour+15*time.Minute, true))
	samples = append(samples, heartbeats(24*time.Hour+20*time.Minute, 48*time.Hour, true)...)

	results := Bucketed(samples, []Window{dayOne, dayTwo}, maxGap)
	if results[0].Unknown != 5*time.Minute {
		t.Fatalf("day one unknown = %s, want the 5m before midnight", results[0].Unknown)
	}
	if results[1].Unknown != 15*time.Minute {
		t.Fatalf("day two unknown = %s, want the 15m after midnight", results[1].Unknown)
	}
	for i, result := range results {
		if result.Up+result.Down+result.Unknown != 24*time.Hour {
			t.Fatalf("day %d does not tile: %#v", i, result)
		}
	}
}

func TestBucketedMatchesPerWindowCompute(t *testing.T) {
	samples := heartbeats(0, 12*time.Hour, true)
	samples = append(samples, heartbeats(12*time.Hour, 30*time.Hour, false)...)
	samples = append(samples, heartbeats(30*time.Hour, 72*time.Hour, true)...)

	windows := []Window{
		{Start: windowStart, End: windowStart.Add(24 * time.Hour)},
		{Start: windowStart.Add(24 * time.Hour), End: windowStart.Add(48 * time.Hour)},
		{Start: windowStart.Add(48 * time.Hour), End: windowStart.Add(72 * time.Hour)},
	}
	bucketed := Bucketed(samples, windows, maxGap)
	for i, window := range windows {
		single := Compute(samples, window, maxGap)
		if bucketed[i] != single {
			t.Fatalf("bucket %d = %#v, single window = %#v", i, bucketed[i], single)
		}
	}
}

func TestCombineAggregatesDurationsNotPercentages(t *testing.T) {
	quiet := Result{Up: 24 * time.Hour}
	broken := Result{Up: 12 * time.Hour, Down: 12 * time.Hour}
	total := Combine(quiet, broken)

	if total.Up != 36*time.Hour || total.Down != 12*time.Hour {
		t.Fatalf("combined = %#v", total)
	}
	if got := percent(t, total); got != 75 {
		t.Fatalf("combined uptime = %v, want 75", got)
	}
}

func TestDailyWindowsCoverExactlyTheRequestedDays(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 3, 17, 40, 0, 0, toronto)
	windows := DailyWindows(now, 90, toronto)

	if len(windows) != 90 {
		t.Fatalf("windows = %d, want 90", len(windows))
	}
	last := windows[len(windows)-1]
	if !last.End.Equal(now) {
		t.Fatalf("last window ends at %s, want now", last.End)
	}
	if last.Start.Format("2006-01-02") != "2026-09-03" {
		t.Fatalf("last window starts on %s, want the current Toronto day", last.Start)
	}
	wantFirst := now.AddDate(0, 0, -89).Format("2006-01-02")
	if windows[0].Start.Format("2006-01-02") != wantFirst {
		t.Fatalf("first window starts on %s, want %s", windows[0].Start, wantFirst)
	}
	for i := 1; i < len(windows); i++ {
		if !windows[i].Start.Equal(windows[i-1].End) {
			t.Fatalf("window %d does not abut its predecessor", i)
		}
	}
}

func TestDailyWindowsSpanDaylightSavingTransition(t *testing.T) {
	toronto, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-11-01 is the fall-back day in Toronto and is twenty-five hours long.
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, toronto)
	windows := DailyWindows(now, 3, toronto)
	fallBack := windows[1]

	if fallBack.Duration() != 25*time.Hour {
		t.Fatalf("fall-back day = %s, want 25h of wall-clock time", fallBack.Duration())
	}
}
