package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecom"
)

func TestMonthRangeRequiresCompletedExactCalendarMonth(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	start, end, e := monthRange("2026-08", now)
	if e != nil || start.Day() != 1 || end.Month() != 9 || end.Sub(start) != 31*24*time.Hour {
		t.Fatal(start, end, e)
	}
	for _, bad := range []string{"2026-10", "2027-01", "2026-8", " 2026-08", "not-a-date"} {
		if _, _, e = monthRange(bad, now); e == nil {
			t.Fatal("bad month accepted", bad)
		}
	}
}

func TestCompletedDateRangeUsesShanghaiInclusiveEndAndSevenDayLimit(t *testing.T) {
	// Midnight in Shanghai: the preceding local date has just completed even
	// though the UTC calendar still says October 28.
	now := time.Date(2026, 10, 28, 16, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, first, last string
		days              int
	}{
		{"night_shift_context", "2026-07-31", "2026-07-31", 1},
		{"seven_days_cross_month", "2026-07-29", "2026-08-04", 7},
		{"last_completed_date", "2026-10-28", "2026-10-28", 1},
		{"cross_year", "2025-12-31", "2026-01-01", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			start, end, e := completedDateRange(tc.first, tc.last, now)
			if e != nil || start.Format(time.RFC3339) != tc.first+"T00:00:00+08:00" || end.Add(-time.Second).Format(time.RFC3339) != tc.last+"T23:59:59+08:00" || end.Sub(start) != time.Duration(tc.days)*24*time.Hour {
				t.Fatal(start, end, e)
			}
		})
	}
	for _, tc := range []struct{ first, last string }{
		{"", "2026-07-31"}, {"2026-07-31", ""},
		{"2026-07-31", "2026-07-30"}, {"2026-07-29", "2026-08-05"},
		{"2026-10-29", "2026-10-29"}, {"2026-10-28", "2026-10-30"},
		{"2026-7-31", "2026-07-31"}, {" 2026-07-31", "2026-07-31"},
		{"2026-07-31", "2026-07-31 "}, {"2026-02-29", "2026-03-01"},
	} {
		if _, _, e := completedDateRange(tc.first, tc.last, now); e == nil {
			t.Fatalf("invalid range accepted: %q to %q", tc.first, tc.last)
		}
	}
	if _, _, e := completedDateRange("2026-10-28", "2026-10-28", now.Add(-time.Nanosecond)); e == nil {
		t.Fatal("date accepted before local midnight")
	}
}

func TestBackfillOptionsRejectMixedAndIncompleteRangesBeforeInitialization(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ month, first, last string }{
		{"2026-08", "2026-07-31", "2026-07-31"},
		{"2026-08", "2026-07-31", ""},
		{"2026-08", "", "2026-07-31"},
		{"", "2026-07-31", ""}, {"", "", "2026-07-31"},
	} {
		if e := validateBackfillOptions(tc.month, tc.first, tc.last, now); e == nil {
			t.Fatal("invalid options accepted", tc)
		}
	}
	for _, tc := range []struct{ month, first, last string }{
		{"", "", ""}, {"2026-08", "", ""}, {"", "2026-07-31", "2026-07-31"},
	} {
		if e := validateBackfillOptions(tc.month, tc.first, tc.last, now); e != nil {
			t.Fatal(tc, e)
		}
	}
}

func TestSingleDateBackfillUsesAllKindsAndPartialSummary(t *testing.T) {
	start, end, e := completedDateRange("2026-07-31", "2026-07-31", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	kinds := []string{"checkin", "leave", "out", "trip", "overtime"}
	calls := 0
	reports := []report{}
	e = backfillWindows(context.Background(), "", start, end, kinds, func(_ context.Context, kind string, from, until time.Time) (string, int, error) {
		if kind != kinds[calls] || from.Format(time.RFC3339) != "2026-07-31T00:00:00+08:00" || until.Format(time.RFC3339) != "2026-07-31T23:59:59+08:00" {
			t.Fatal(kind, from, until)
		}
		calls++
		if kind == "leave" {
			return "partial", 2, &wecom.PartialError{Failed: 1}
		}
		return "ok", 2, nil
	}, func(r report) { reports = append(reports, r) })
	var partial *wecom.PartialError
	if !errors.As(e, &partial) || partial.Failed != 1 || calls != 5 || len(reports) != 6 {
		t.Fatal(e, calls, reports)
	}
	summary := reports[len(reports)-1]
	if summary.Month != "" || summary.Start != "2026-07-31T00:00:00+08:00" || summary.End != "2026-07-31T23:59:59+08:00" || summary.Jobs != 5 || summary.Count != 10 || summary.Failed != 1 || summary.Status != "partial" || summary.OK {
		t.Fatal(summary)
	}
}

func TestBootstrapErrorsDoNotContainDatabaseOrUpstreamSecrets(t *testing.T) {
	r := failure("db", errors.New("root:secret-password@tcp(127.0.0.1:3306)/database"))
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "root") || strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "3306") {
		t.Fatal(string(raw))
	}
	r = failure("api", &wecom.APIError{Code: 48009, Endpoint: "/checkin/getcheckindata"})
	if r.ErrorCode != 48009 || r.OK {
		t.Fatal(r)
	}
}

func TestBackfillPartialContinuesKindsAndWindowsAndReturnsFailureSummary(t *testing.T) {
	start, end, e := monthRange("2026-08", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	calls := 0
	reports := []report{}
	kinds := []string{}
	execute := func(ctx context.Context, kind string, from, until time.Time) (string, int, error) {
		calls++
		kinds = append(kinds, kind)
		if until.Sub(from) > 7*24*time.Hour || until.Before(from) {
			t.Fatal("invalid window", from, until)
		}
		if calls == 1 {
			return "partial-first", 7, fmt.Errorf("must-not-print-password: %w", &wecom.PartialError{Failed: 3})
		}
		if calls == 4 {
			return "partial-next", 7, &wecom.PartialError{Failed: 2}
		}
		return "ok", 7, nil
	}
	e = backfillWindows(context.Background(), "2026-08", start, end, []string{"checkin", "leave"}, execute, func(r report) { reports = append(reports, r) })
	var partial *wecom.PartialError
	if !errors.As(e, &partial) || partial.Failed != 5 || calls != 10 || len(reports) != 11 {
		t.Fatal(e, calls, reports)
	}
	if kinds[0] != "checkin" || kinds[1] != "leave" || reports[0].Status != "partial" || reports[0].Failed != 3 || reports[3].Failed != 2 {
		t.Fatal(kinds, reports)
	}
	summary := reports[len(reports)-1]
	if summary.Action != "backfill-summary" || summary.OK || summary.Status != "partial" || summary.Failed != 5 || summary.Count != 70 || summary.Jobs != 10 || summary.PartialJobs != 2 {
		t.Fatal(summary)
	}
	raw, _ := json.Marshal(reports)
	if strings.Contains(string(raw), "must-not-print-password") {
		t.Fatal("wrapped error leaked")
	}
}

func TestBackfillFatalStopsAfterEarlierPartialAndStillSummarizes(t *testing.T) {
	start, end, _ := monthRange("2026-08", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	calls := 0
	reports := []report{}
	fatal := errors.New("root:private-db-password@tcp(127.0.0.1:3306)")
	execute := func(context.Context, string, time.Time, time.Time) (string, int, error) {
		calls++
		if calls == 1 {
			return "partial", 4, &wecom.PartialError{Failed: 2}
		}
		return "fatal", 0, fatal
	}
	e := backfillWindows(context.Background(), "2026-08", start, end, []string{"checkin", "leave"}, execute, func(r report) { reports = append(reports, r) })
	if !errors.Is(e, fatal) || calls != 2 || len(reports) != 3 {
		t.Fatal(e, calls, reports)
	}
	summary := reports[2]
	if summary.Status != "failed" || summary.OK || summary.Failed != 2 || summary.Count != 4 || summary.Jobs != 2 || summary.PartialJobs != 1 {
		t.Fatal(summary)
	}
	raw, _ := json.Marshal(reports)
	if strings.Contains(string(raw), "private-db-password") || strings.Contains(string(raw), "3306") {
		t.Fatal("fatal error leaked")
	}
}

func TestBackfillSuccessProducesSuccessfulSummary(t *testing.T) {
	start, end, _ := monthRange("2026-09", time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	reports := []report{}
	e := backfillWindows(context.Background(), "2026-09", start, end, []string{"checkin"}, func(context.Context, string, time.Time, time.Time) (string, int, error) { return "id", 2, nil }, func(r report) { reports = append(reports, r) })
	if e != nil || len(reports) != 6 {
		t.Fatal(e, reports)
	}
	summary := reports[5]
	if !summary.OK || summary.Status != "success" || summary.Count != 10 || summary.Jobs != 5 || summary.Failed != 0 {
		t.Fatal(summary)
	}
}
