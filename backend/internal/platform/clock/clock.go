// Package clock builds the "now" function the services share. Calendar days
// (today, the current month or cycle, a future period) follow the configured
// zone (TZ_NAME), never the container's UTC; instants stored as created_at stay
// UTC because only the calendar day depends on the zone.
package clock

import "time"

// In returns a clock that reports the current instant in loc, so formatting it
// as a date or a month yields the local calendar day.
func In(loc *time.Location) func() time.Time {
	return func() time.Time { return time.Now().In(loc) }
}

// Fixed returns a clock frozen at instant, expressed in loc (tests).
func Fixed(instant time.Time, loc *time.Location) func() time.Time {
	t := instant.In(loc)
	return func() time.Time { return t }
}
