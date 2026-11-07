package attendanceengine

import "testing"

func TestRosterGenerationAndExplicitResolution(t *testing.T) {
	cycle := Cycle{Anchor: "2026-10-29", Days: []string{"day", "night", ""}}
	id, err := ResolveCycle(cycle.Anchor, "2026-10-28", cycle.Days)
	if err != nil || id != "" {
		t.Fatalf("negative cycle %q %v", id, err)
	}
	days, err := Generate(GenerateInput{Start: "2026-10-29", End: "2026-11-01", Cycle: cycle, Calendar: map[string]RosterDay{"2026-10-30": {Category: Holiday}}})
	if err != nil || len(days) != 4 || days[0].ShiftID != "day" || days[1].Category != Holiday || days[2].Category != Restday || days[3].ShiftID != "day" || days[0].Confirmed {
		t.Fatalf("%+v %v", days, err)
	}
	if _, err = ResolveRoster("2026-10-29", []RosterCandidate{{Day: days[0], Priority: 1}}); err == nil {
		t.Fatal("draft roster treated as confirmed")
	}
	days[0].Confirmed = true
	resolved, err := ResolveRoster("2026-10-29", []RosterCandidate{{Day: days[0], Priority: 1}})
	if err != nil || resolved.ShiftID != "day" {
		t.Fatal(resolved, err)
	}
	conflict := days[0]
	conflict.ShiftID = "night"
	if _, err = ResolveRoster("2026-10-29", []RosterCandidate{{Day: days[0], Priority: 1}, {Day: conflict, Priority: 1}}); err == nil {
		t.Fatal("equal priority conflict accepted")
	}
	// Shanghai had DST: calendar date cycling must not use elapsed 24h chunks.
	id, err = ResolveCycle("1991-04-14", "1991-04-15", []string{"a", "b"})
	if err != nil || id != "b" {
		t.Fatal(id, err)
	}
	if _, err = ParseDate("2026-02-29"); err == nil {
		t.Fatal("invalid date")
	}
	if _, err = ParseDate("2028-02-29"); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidRulesCannotCalculate(t *testing.T) {
	in := baseline()
	in.Rule.Window.InAfterMinutes = -1
	if ValidateRule(in.Rule) == nil || Calculate(in).State != "review" {
		t.Fatal("invalid window published")
	}
	in = baseline()
	in.Shift.Segments = append(in.Shift.Segments, Segment{ID: "duplicate-work", Kind: "work", StartMinute: 600, EndMinute: 620})
	if ValidateShift(in.Shift) == nil {
		t.Fatal("overlapping work accepted")
	}
	in = baseline()
	in.Shift.Segments[0].EndMinute = 500
	if Calculate(in).State != "review" {
		t.Fatal("reverse overnight silently guessed")
	}
	in = baseline()
	in.AsOf = 0
	if Calculate(in).State != "review" {
		t.Fatal("implicit wall clock used")
	}
}
