package common

import "time"

// DateAfter reports whether the calendar date of value falls after the
// calendar date of date. Both are wall-clock values, so only their year,
// month, and day count; the time of day and the location are ignored.
func DateAfter(value, date time.Time) bool {
	valueDate := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	dateOnly := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	return valueDate.After(dateOnly)
}
