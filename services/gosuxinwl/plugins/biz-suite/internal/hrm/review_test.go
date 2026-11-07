package hrm

import "testing"

// A Tuesday-only shift must not consume Monday's late arrival merely because
// its wall-clock start is closer to the punch.
func TestReviewUnscheduledShiftMustNotStealPunch(t *testing.T) {
	shifts := []Shift{{Start: "09:00", End: "18:00", Weekdays: []int{1}}, {Start: "10:00", End: "19:00", Weekdays: []int{2}}}
	clocks := []Clock{{ClockTime: "2026-01-05 09:50:00", ClockType: "in"}, {ClockTime: "2026-01-05 18:00:00", ClockType: "out"}}
	day := calculateDay(tm("2026-01-05"), shifts, Rule{}, clocks, nil, nil, nil, tm("2026-01-07"))
	if day.MissingCount != 0 || day.LateCount != 1 || !day.Worked {
		t.Fatalf("Tuesday schedule stole Monday attendance: %+v", day)
	}
}
