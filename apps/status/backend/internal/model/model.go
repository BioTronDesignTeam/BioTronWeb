package model

import "time"

type Health struct {
	Service   string    `json:"service"`
	OK        bool      `json:"ok"`
	Detail    string    `json:"detail,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// HealthPoint is one recorded health observation stripped of the operator-only
// detail string. The public status layer must never see Health.Detail, which
// carries raw dial errors and internal hostnames, so history queries return
// this narrower shape instead.
type HealthPoint struct {
	OK        bool
	CheckedAt time.Time
	// Continuous records that observation carried on without interruption from
	// this point until the next point in the slice. The history query collapses
	// long runs of identical heartbeats into their endpoints, which throws away
	// the evidence that anybody was watching in between; this flag carries that
	// evidence across the collapse. False means "no such promise", so a caller
	// that reads every row back falls through to judging the gap by its length,
	// which is what an uncollapsed slice needs.
	Continuous bool
}
