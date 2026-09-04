package uptime

import (
	"testing"
	"time"

	"github.com/BioTronDesignTeam/Logger/backend/internal/model"
)

// collapse is a faithful Go transcription of the row-emitting rule in
// Store.HealthHistory. Keeping an executable copy of the SQL's semantics here is
// what lets the walker's behaviour on a collapsed slice be compared against its
// behaviour on the full one; if the SQL's emission rule ever changes, this must
// change with it.
//
// A row survives when it is a state transition, when it ends a silence, when it
// starts one, or when it is the last row in the window. A surviving row is
// marked continuous when observation carried on to the next row within the
// tolerance, which is the fact the collapse would otherwise destroy.
func collapse(samples []model.HealthPoint, tolerance time.Duration) []model.HealthPoint {
	kept := make([]model.HealthPoint, 0, len(samples))
	for i, sample := range samples {
		transition := i == 0 || samples[i-1].OK != sample.OK
		endsSilence := i > 0 && sample.CheckedAt.Sub(samples[i-1].CheckedAt) > tolerance
		startsSilence := i+1 < len(samples) && samples[i+1].CheckedAt.Sub(sample.CheckedAt) > tolerance
		last := i == len(samples)-1
		if !transition && !endsSilence && !startsSilence && !last {
			continue
		}
		sample.Continuous = i+1 < len(samples) && samples[i+1].CheckedAt.Sub(sample.CheckedAt) <= tolerance
		kept = append(kept, sample)
	}
	return kept
}

// ninetyDaySeries reproduces the shape of real history: five-minute heartbeats
// across ninety days, with one outage and one stretch where Logger itself was
// not running.
func ninetyDaySeries(start time.Time) []model.HealthPoint {
	outageFrom := start.Add(20 * 24 * time.Hour)
	outageTo := outageFrom.Add(70 * time.Minute)
	silenceFrom := start.Add(40 * 24 * time.Hour)
	silenceTo := silenceFrom.Add(6 * time.Hour)

	samples := make([]model.HealthPoint, 0, 26000)
	for at := start; at.Before(start.Add(90 * 24 * time.Hour)); at = at.Add(5 * time.Minute) {
		if !at.Before(silenceFrom) && at.Before(silenceTo) {
			continue // nothing was recorded, because nothing was watching
		}
		down := !at.Before(outageFrom) && at.Before(outageTo)
		samples = append(samples, model.HealthPoint{OK: !down, CheckedAt: at})
	}
	return samples
}

// TestCollapsedSamplesYieldIdenticalUptime is the property the whole server-side
// collapse rests on: the walker must not be able to tell the difference.
func TestCollapsedSamplesYieldIdenticalUptime(t *testing.T) {
	start := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	full := ninetyDaySeries(start)
	collapsed := collapse(full, maxGap)

	if len(collapsed) >= len(full)/100 {
		t.Fatalf("collapse kept %d of %d rows, expected a drastic reduction", len(collapsed), len(full))
	}
	t.Logf("collapsed %d rows to %d", len(full), len(collapsed))

	windows := DailyWindows(start.Add(90*24*time.Hour), 90, time.UTC)
	windows = append(windows, Window{Start: start, End: start.Add(90 * 24 * time.Hour)})

	for i, window := range windows {
		fromFull := Compute(full, window, maxGap)
		fromCollapsed := Compute(collapsed, window, maxGap)
		if fromFull != fromCollapsed {
			t.Fatalf("window %d (%s): full = %#v, collapsed = %#v", i, window.Start, fromFull, fromCollapsed)
		}
	}
}

// TestCollapseWithoutContinuityWouldLoseObservedTime pins down why the flag has
// to exist. Dropping it is not a cosmetic simplification: ninety days of proven
// observation collapse to the few minutes that happen to sit next to a kept row,
// and the outage disappears entirely because a seventy-minute down segment is
// itself longer than the tolerance.
func TestCollapseWithoutContinuityWouldLoseObservedTime(t *testing.T) {
	start := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	full := ninetyDaySeries(start)
	window := Window{Start: start, End: start.Add(90 * 24 * time.Hour)}

	stripped := collapse(full, maxGap)
	for i := range stripped {
		stripped[i].Continuous = false
	}
	honest := Compute(full, window, maxGap)
	lossy := Compute(stripped, window, maxGap)

	if honest.Observed() < 89*24*time.Hour {
		t.Fatalf("full series observed = %s, want nearly the whole window", honest.Observed())
	}
	if lossy.Observed() > time.Hour {
		t.Fatalf("stripped series observed = %s, want almost nothing left", lossy.Observed())
	}
	if lossy.Down != 0 {
		t.Fatalf("stripped series down = %s, want the outage lost as unknown", lossy.Down)
	}
	if honest.Down != 70*time.Minute {
		t.Fatalf("full series down = %s, want the injected outage", honest.Down)
	}
}

// TestContinuityDoesNotHideRealSilence guards the other direction: the flag must
// never paper over a stretch nobody observed.
func TestContinuityDoesNotHideRealSilence(t *testing.T) {
	start := time.Date(2026, 6, 5, 13, 0, 0, 0, time.UTC)
	full := ninetyDaySeries(start)
	collapsed := collapse(full, maxGap)
	window := Window{Start: start, End: start.Add(90 * 24 * time.Hour)}

	result := Compute(collapsed, window, maxGap)
	// The silence starts at the last heartbeat before it, not at the first
	// missing one, so it measures one heartbeat longer than the hole itself.
	if result.Unknown != 6*time.Hour+5*time.Minute {
		t.Fatalf("unknown = %s, want the injected six-hour silence", result.Unknown)
	}
	if result.Down != 70*time.Minute {
		t.Fatalf("down = %s, want the injected seventy-minute outage", result.Down)
	}
}

// TestCollapseIsLosslessAcrossToleranceBoundaries exercises the awkward cases:
// gaps just under and just over the tolerance, and back-to-back transitions.
func TestCollapseIsLosslessAcrossToleranceBoundaries(t *testing.T) {
	start := time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)
	offsets := []time.Duration{
		0, 5 * time.Minute, 10 * time.Minute,
		// A gap of exactly the tolerance is still observed.
		25 * time.Minute,
		30 * time.Minute,
		// One second over the tolerance is a silence.
		45*time.Minute + time.Second,
		50 * time.Minute, 55 * time.Minute,
	}
	states := []bool{true, true, true, false, false, false, true, true}

	full := make([]model.HealthPoint, 0, len(offsets))
	for i, offset := range offsets {
		full = append(full, model.HealthPoint{OK: states[i], CheckedAt: start.Add(offset)})
	}
	collapsed := collapse(full, maxGap)
	window := Window{Start: start, End: start.Add(time.Hour)}

	if Compute(full, window, maxGap) != Compute(collapsed, window, maxGap) {
		t.Fatalf("full = %#v, collapsed = %#v", Compute(full, window, maxGap), Compute(collapsed, window, maxGap))
	}
}
