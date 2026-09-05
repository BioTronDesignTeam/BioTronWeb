// Package uptime turns sparse health-check samples into time-weighted
// availability figures.
//
// The monitor only persists a row when a component changes state or when the
// heartbeat interval elapses, so rows are unevenly spaced. Counting rows would
// over-weight flapping periods, where transitions cluster densely, and produce
// a number that is simply wrong. Every figure here is therefore weighted by the
// wall-clock duration each observed state held.
//
// The package deliberately depends on nothing but the standard library and the
// model types, so the maths is testable without Postgres, Redis, or HTTP.
package uptime

import (
	"math"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// Window is a half-open interval [Start, End).
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration is the wall-clock length of the window, never negative.
func (w Window) Duration() time.Duration {
	if !w.End.After(w.Start) {
		return 0
	}
	return w.End.Sub(w.Start)
}

// Result is the time-weighted breakdown of one window. Up, Down, and Unknown
// always sum to the window duration.
type Result struct {
	Up      time.Duration
	Down    time.Duration
	Unknown time.Duration
}

// Observed is the time the component was actually being watched.
func (r Result) Observed() time.Duration { return r.Up + r.Down }

// Percent is the availability of the window as a percentage rounded to two
// decimals, or nil when nothing was observed. A window nobody was watching is
// honestly unknown rather than optimistically 100.
//
// The value is clamped to 99.99 whenever any downtime was observed, so a green
// "100%" is never a lie about a window that contained an outage.
func (r Result) Percent() *float64 {
	observed := r.Observed()
	if observed <= 0 {
		return nil
	}
	value := math.Round(float64(r.Up)/float64(observed)*10000) / 100
	if r.Down > 0 && value >= 100 {
		value = 99.99
	}
	if value > 100 {
		value = 100
	}
	if value < 0 {
		value = 0
	}
	return &value
}

// UnknownRatio is the fraction of the window for which no observation exists,
// so callers can say "no data" instead of inventing availability.
func (r Result) UnknownRatio(window time.Duration) float64 {
	if window <= 0 {
		return 0
	}
	return float64(r.Unknown) / float64(window)
}

// segmentKind classifies the state that held across one stretch of time.
type segmentKind int

const (
	kindUnknown segmentKind = iota
	kindUp
	kindDown
)

// Compute is the single-window form of Bucketed.
func Compute(samples []model.HealthPoint, window Window, maxGap time.Duration) Result {
	results := Bucketed(samples, []Window{window}, maxGap)
	return results[0]
}

// Bucketed walks the samples once and spreads every observed segment across the
// windows it overlaps. Windows must be ascending and non-overlapping; the
// returned slice has one Result per window.
//
// The algorithm is the one described in the status-page contract:
//
//  1. Each sample's state holds from its timestamp until the next sample's
//     timestamp; the final sample holds until the end of the last window.
//  2. Segments are clipped to each window, so a state carried in from before a
//     window contributes only the part inside it.
//  3. A segment longer than maxGap means nobody was watching — Logger itself
//     was probably down — so it counts as unknown and is excluded from both the
//     numerator and the denominator instead of silently inventing uptime. A
//     sample marked Continuous is exempt: the store has already established that
//     observation ran unbroken from it to the next sample, and collapsing a long
//     run of identical heartbeats into its endpoints is exactly what makes an
//     observed stretch look like a long silence.
//
// The gap test uses the full, unclipped distance between consecutive
// observations rather than the clipped part. A twenty-minute observation gap
// spanning midnight is one gap, and it must be judged the same way in both
// daily buckets it touches.
func Bucketed(samples []model.HealthPoint, windows []Window, maxGap time.Duration) []Result {
	results := make([]Result, len(windows))
	if len(windows) == 0 {
		return results
	}
	rangeStart := windows[0].Start
	rangeEnd := windows[len(windows)-1].End
	if !rangeEnd.After(rangeStart) {
		return results
	}

	spread := spreader{results: results, windows: windows}
	ordered := relevant(samples, rangeEnd)
	if len(ordered) == 0 {
		// Nothing was ever recorded, so the whole range is unknown.
		spread.add(rangeStart, rangeEnd, kindUnknown)
		return results
	}

	// Time before the first sample was never observed for this component.
	if ordered[0].CheckedAt.After(rangeStart) {
		spread.add(rangeStart, ordered[0].CheckedAt, kindUnknown)
	}

	for i, sample := range ordered {
		segmentEnd := rangeEnd
		if i+1 < len(ordered) {
			segmentEnd = ordered[i+1].CheckedAt
		}
		if !segmentEnd.After(sample.CheckedAt) {
			continue
		}
		kind := kindDown
		if sample.OK {
			kind = kindUp
		}
		if !sample.Continuous && segmentEnd.Sub(sample.CheckedAt) > maxGap {
			kind = kindUnknown
		}
		spread.add(sample.CheckedAt, segmentEnd, kind)
	}
	return results
}

// relevant drops samples recorded after the range and everything but the most
// recent sample at or before the range start, which is the state the component
// was already in when the range opened. Input is assumed ascending by
// CheckedAt, which is how the store returns it.
func relevant(samples []model.HealthPoint, rangeEnd time.Time) []model.HealthPoint {
	ordered := make([]model.HealthPoint, 0, len(samples))
	for _, sample := range samples {
		if sample.CheckedAt.After(rangeEnd) {
			continue
		}
		ordered = append(ordered, sample)
	}
	return ordered
}

// spreader clips segments into the windows they overlap. Segments arrive in
// ascending order, so the cursor only ever moves forward and the whole walk
// stays linear in samples plus windows rather than multiplying them.
type spreader struct {
	results []Result
	windows []Window
	cursor  int
}

func (s *spreader) add(start, end time.Time, kind segmentKind) {
	for s.cursor < len(s.windows) && !s.windows[s.cursor].End.After(start) {
		s.cursor++
	}
	for i := s.cursor; i < len(s.windows); i++ {
		window := s.windows[i]
		if !end.After(window.Start) {
			break
		}
		from := start
		if window.Start.After(from) {
			from = window.Start
		}
		to := end
		if window.End.Before(to) {
			to = window.End
		}
		if !to.After(from) {
			continue
		}
		duration := to.Sub(from)
		switch kind {
		case kindUp:
			s.results[i].Up += duration
		case kindDown:
			s.results[i].Down += duration
		default:
			s.results[i].Unknown += duration
		}
	}
}

// Combine merges per-component results into one aggregate. Aggregating the
// durations rather than averaging the percentages keeps components with partial
// history from distorting the headline figure.
func Combine(results ...Result) Result {
	var total Result
	for _, result := range results {
		total.Up += result.Up
		total.Down += result.Down
		total.Unknown += result.Unknown
	}
	return total
}

// DailyWindows returns one window per calendar day in loc, oldest first and
// exactly days long, ending with the day containing now. The final window stops
// at now rather than at midnight, because the rest of today has not happened.
func DailyWindows(now time.Time, days int, loc *time.Location) []Window {
	if days < 1 {
		days = 1
	}
	local := now.In(loc)
	todayStart := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	windows := make([]Window, 0, days)
	for offset := days - 1; offset >= 0; offset-- {
		start := todayStart.AddDate(0, 0, -offset)
		end := start.AddDate(0, 0, 1)
		if end.After(now) {
			end = now
		}
		windows = append(windows, Window{Start: start, End: end})
	}
	return windows
}
