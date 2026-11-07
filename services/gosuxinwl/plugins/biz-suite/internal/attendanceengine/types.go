// Package attendanceengine calculates attendance without database or network IO.
// Dates use Asia/Shanghai; instants are Unix seconds and durations are integers.
package attendanceengine

const Version = "attendance-v2-local"

type Category string

const (
	Workday Category = "workday"
	Restday Category = "restday"
	Holiday Category = "holiday"
)

// A segment's minutes are offsets from the work date's local midnight. For a
// 20:00–08:00 overnight segment, use 1200 and 1920, rather than a negative range.
type Segment struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	Kind                string             `json:"kind"` // work, break, overtime
	StartMinute         int                `json:"start_minute"`
	EndMinute           int                `json:"end_minute"`
	PunchMode           string             `json:"punch_mode"` // both(default), in, out, none
	Window              *PunchWindow       `json:"window,omitempty"`
	LocalWindows        *LocalPunchWindows `json:"local_windows,omitempty"`
	EvidenceID          string             `json:"evidence_id,omitempty"`
	OpenEnded           bool               `json:"open_ended"`            // overtime only: actual out punch determines end; EndMinute is the search horizon
	OvertimeStartMinute int                `json:"overtime_start_minute"` // 0 uses segment start; excludes uncredited return-to-work lead-in
}

type Shift struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Version          string           `json:"version"`
	Segments         []Segment        `json:"segments"`
	SourceGroupID    string           `json:"source_group_id,omitempty"`
	SourceScheduleID string           `json:"source_schedule_id,omitempty"`
	OvertimeVariant  *ShiftVariant    `json:"overtime_variant,omitempty"`
	EvidencePeriods  []EvidencePeriod `json:"evidence_periods,omitempty"`
}

// Local windows are half-open offsets in seconds from the work-date midnight.
// Nil means unconfigured; zero is the valid midnight boundary.
type LocalPunchWindows struct {
	InStartSecond  *int `json:"in_start_second"`
	InEndSecond    *int `json:"in_end_second"`
	OutStartSecond *int `json:"out_start_second"`
	OutEndSecond   *int `json:"out_end_second"`
}

func (w *LocalPunchWindows) Bounds() (start, end int, ok bool) {
	if w == nil {
		return
	}
	for _, pair := range [][2]*int{{w.InStartSecond, w.InEndSecond}, {w.OutStartSecond, w.OutEndSecond}} {
		if pair[0] == nil || pair[1] == nil {
			continue
		}
		if !ok {
			start, end, ok = *pair[0], *pair[1], true
		} else {
			start = min(start, *pair[0])
			end = max(end, *pair[1])
		}
	}
	return
}

// Evidence periods describe physical punch obligations independently of the
// work/OT metric slices which refer to their ID.
type EvidencePeriod struct {
	ID           string             `json:"id"`
	StartSecond  int                `json:"start_second"`
	EndSecond    int                `json:"end_second"`
	PunchMode    string             `json:"punch_mode"`
	LocalWindows *LocalPunchWindows `json:"local_windows"`
}

type LocalTimeRange struct {
	StartSecond int `json:"start_second"`
	EndSecond   int `json:"end_second"`
}

// A duration-only meal allowance cannot invent a meal's time interval. Known
// partial ranges may use actual time; other incomplete days remain for review.
type PartialWorkPolicy struct {
	Mode         string           `json:"mode"`
	ActualRanges []LocalTimeRange `json:"actual_ranges,omitempty"`
}

// A variant is an explicitly configured conditional shift, never a guess from
// arbitrary punches. TriggerMinute is the earliest evening return-to-work time.
type ShiftVariant struct {
	TriggerMinute int   `json:"trigger_minute"`
	Shift         Shift `json:"shift"`
}

type PunchWindow struct {
	InBeforeMinutes  int `json:"in_before_minutes"`
	InAfterMinutes   int `json:"in_after_minutes"`
	OutBeforeMinutes int `json:"out_before_minutes"`
	OutAfterMinutes  int `json:"out_after_minutes"`
}

type OvertimePolicy struct {
	Enabled                bool   `json:"enabled"`
	RequireApproval        bool   `json:"require_approval"`
	MinimumMinutes         int    `json:"minimum_minutes"`
	RoundingMinutes        int    `json:"rounding_minutes"`         // 0 defaults to 30
	RoundingScope          string `json:"rounding_scope"`           // daily(default), segment
	MaximumMinutes         int    `json:"maximum_minutes"`          // 0 means no explicit cap
	CompleteWorkdayMinutes int    `json:"complete_workday_minutes"` // restday optional work only; 0 preserves ordinary actual-duration rounding
}

type LateArrivalTier struct {
	PreviousOvertimeMinutes int `json:"previous_overtime_minutes"`
	DelayMinutes            int `json:"delay_minutes"`
}

type FullAttendancePolicy struct {
	AllowedLeaveTypes []string `json:"allowed_leave_types"`
	AllowOut          bool     `json:"allow_out"`
	AllowTrip         bool     `json:"allow_trip"`
	AllowLate         bool     `json:"allow_late"`
	AllowEarly        bool     `json:"allow_early"`
	AllowMissing      bool     `json:"allow_missing"`
}

type LeavePolicy struct {
	CreditAttendance      bool `json:"credit_attendance"`
	FullAttendanceAllowed bool `json:"full_attendance_allowed"`
}

type Rule struct {
	EvidenceMode                 string                      `json:"evidence_mode,omitempty"`
	PartialWorkPolicy            *PartialWorkPolicy          `json:"partial_work_policy,omitempty"`
	CompleteWorkDeductionMinutes int                         `json:"complete_work_deduction_minutes,omitempty"`
	LateDepartureAfterSecond     *int                        `json:"late_departure_after_second,omitempty"`
	LateDepartureDelayMinutes    int                         `json:"late_departure_delay_minutes,omitempty"`
	ID                           string                      `json:"id"`
	Version                      string                      `json:"version"`
	Window                       PunchWindow                 `json:"window"`
	PunchSelection               string                      `json:"punch_selection"` // first_last(default), earliest, latest, nearest
	LateGraceMinutes             int                         `json:"late_grace_minutes"`
	EarlyGraceMinutes            int                         `json:"early_grace_minutes"`
	GraceMode                    string                      `json:"grace_mode"`          // normal(default), grace
	AttendanceMode               string                      `json:"attendance_mode"`     // actual(default), fixed(legacy), fixed_complete
	StandardMinutes              int                         `json:"standard_minutes"`    // 0 uses scheduled work, excluding breaks
	LateDeductionMode            string                      `json:"late_deduction_mode"` // none(default), excess, full
	EarlyDeductionMode           string                      `json:"early_deduction_mode"`
	CreditOut                    bool                        `json:"credit_out"`
	CreditTrip                   bool                        `json:"credit_trip"`
	LeavePolicies                map[string]LeavePolicy      `json:"leave_policies"`
	OvertimeDateMode             string                      `json:"overtime_date_mode"` // workdate(default), calendar
	FlexMinutes                  int                         `json:"flex_minutes"`
	LateArrivalTiers             []LateArrivalTier           `json:"late_arrival_tiers"`
	AllowAbnormalPunches         bool                        `json:"allow_abnormal_punches"`
	Overtime                     map[Category]OvertimePolicy `json:"overtime"`
	FullAttendance               FullAttendancePolicy        `json:"full_attendance"`
}

type Punch struct {
	Placeholder      bool   `json:"placeholder,omitempty"`
	ID               string `json:"id"`
	At               int64  `json:"at"`
	Type             string `json:"type"` // in, out, outside, unknown
	Status           string `json:"status"`
	ScheduledAt      int64  `json:"scheduled_at,omitempty"`
	SourceGroupID    string `json:"source_group_id,omitempty"`
	SourceScheduleID string `json:"source_schedule_id,omitempty"`
}

type Interval struct {
	StartAt int64 `json:"start_at"`
	EndAt   int64 `json:"end_at"`
}

// Slices contains verified per-day/half-day intervals when supplied by the
// approval adapter. The engine never guesses a working day from a unit string.
type Absence struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"` // leave, out, trip, overtime
	LeaveType string     `json:"leave_type,omitempty"`
	Status    string     `json:"status"`
	StartAt   int64      `json:"start_at"`
	EndAt     int64      `json:"end_at"`
	Unit      string     `json:"unit,omitempty"`
	Slices    []Interval `json:"slices,omitempty"`
}

// Adjustments are separate approved evidence; source punches remain unchanged.
type Adjustment struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // credit, absence, waive_late, waive_early, waive_missing, exclude_punch
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	SegmentID string `json:"segment_id,omitempty"`
	PunchID   string `json:"punch_id,omitempty"`
	StartAt   int64  `json:"start_at,omitempty"`
	EndAt     int64  `json:"end_at,omitempty"`
}

type Input struct {
	EmployeeID              string              `json:"employee_id"`
	Date                    string              `json:"date"`
	Category                Category            `json:"category"`
	ScheduledWork           bool                `json:"scheduled_work"` // rest/holiday only: explicitly required work; otherwise optional OT template
	DayCategories           map[string]Category `json:"day_categories,omitempty"`
	Rule                    Rule                `json:"rule"`
	Shift                   Shift               `json:"shift"`
	Punches                 []Punch             `json:"punches"`
	Absences                []Absence           `json:"absences"`
	Adjustments             []Adjustment        `json:"adjustments"`
	PreviousDate            string              `json:"previous_date,omitempty"`
	PreviousOvertimeMinutes int                 `json:"previous_overtime_minutes"`
	PreviousOutAt           int64               `json:"previous_out_at,omitempty"`
	AsOf                    int64               `json:"as_of"` // caller's fixed calculation instant
}

type Trace struct {
	Code        string   `json:"code"`
	Message     string   `json:"message"`
	SegmentID   string   `json:"segment_id,omitempty"`
	EvidenceIDs []string `json:"evidence_ids,omitempty"`
	Seconds     int64    `json:"seconds,omitempty"`
}

type SegmentResult struct {
	PresenceSeconds        int64    `json:"presence_seconds"`
	DeductedSeconds        int64    `json:"deducted_seconds"`
	ID                     string   `json:"id"`
	Kind                   string   `json:"kind"`
	StartAt                int64    `json:"start_at"`
	EndAt                  int64    `json:"end_at"`
	InPunchID              string   `json:"in_punch_id,omitempty"`
	OutPunchID             string   `json:"out_punch_id,omitempty"`
	ExpectedSeconds        int64    `json:"expected_seconds"`
	ActualSeconds          int64    `json:"actual_seconds"`
	CreditedSeconds        int64    `json:"credited_seconds"`
	LeaveSeconds           int64    `json:"leave_seconds"`
	OutSeconds             int64    `json:"out_seconds"`
	TripSeconds            int64    `json:"trip_seconds"`
	ConfirmedAbsentSeconds int64    `json:"confirmed_absent_seconds"`
	LateSeconds            int64    `json:"late_seconds"`
	EarlySeconds           int64    `json:"early_seconds"`
	RawLateSeconds         int64    `json:"raw_late_seconds"`
	RawEarlySeconds        int64    `json:"raw_early_seconds"`
	GraceSeconds           int64    `json:"grace_seconds"`
	MissingCount           int      `json:"missing_count"`
	RawOvertimeSeconds     int64    `json:"raw_overtime_seconds"`
	OvertimeSeconds        int64    `json:"overtime_seconds"`
	State                  string   `json:"state"` // calculated, pending, review
	Issues                 []string `json:"issues"`
}

type OvertimeBucket struct {
	Date                   string   `json:"date"`
	Category               Category `json:"category"`
	RawSeconds             int64    `json:"raw_seconds"`
	Seconds                int64    `json:"seconds"`
	WorkdayActualSeconds   int64    `json:"workday_actual_seconds,omitempty"`
	WorkdayCreditedSeconds int64    `json:"workday_credited_seconds,omitempty"`
	AdditionalSeconds      int64    `json:"additional_seconds,omitempty"`
	CompleteWorkdayApplied bool     `json:"complete_workday_applied,omitempty"`
}

type Result struct {
	PresenceSeconds        int64             `json:"presence_seconds"`
	DeductedSeconds        int64             `json:"deducted_seconds"`
	PunchAssignments       []PunchAssignment `json:"punch_assignments,omitempty"`
	EngineVersion          string            `json:"engine_version"`
	EmployeeID             string            `json:"employee_id"`
	Date                   string            `json:"date"`
	Category               Category          `json:"category"`
	RuleVersion            string            `json:"rule_version"`
	ShiftVersion           string            `json:"shift_version"`
	State                  string            `json:"state"` // calculated, pending, review, rest
	ExpectedSeconds        int64             `json:"expected_seconds"`
	StandardSeconds        int64             `json:"standard_seconds"`
	ActualSeconds          int64             `json:"actual_seconds"`
	CreditedSeconds        int64             `json:"credited_seconds"`
	LeaveSeconds           int64             `json:"leave_seconds"`
	OutSeconds             int64             `json:"out_seconds"`
	TripSeconds            int64             `json:"trip_seconds"`
	ConfirmedAbsentSeconds int64             `json:"confirmed_absent_seconds"`
	LateSeconds            int64             `json:"late_seconds"`
	EarlySeconds           int64             `json:"early_seconds"`
	RawLateSeconds         int64             `json:"raw_late_seconds"`
	RawEarlySeconds        int64             `json:"raw_early_seconds"`
	GraceSeconds           int64             `json:"grace_seconds"`
	LateCount              int               `json:"late_count"`
	EarlyCount             int               `json:"early_count"`
	MissingCount           int               `json:"missing_count"`
	RawOvertimeSeconds     int64             `json:"raw_overtime_seconds"`
	OvertimeSeconds        int64             `json:"overtime_seconds"`
	OvertimeBuckets        []OvertimeBucket  `json:"overtime_buckets"`
	FlexAppliedMinutes     int               `json:"flex_applied_minutes"`
	FlexAppliedSeconds     int64             `json:"flex_applied_seconds"`
	LateArrivalMinutes     int               `json:"late_arrival_minutes"`
	FullAttendanceEligible bool              `json:"full_attendance_eligible"`
	FullAttendanceReasons  []string          `json:"full_attendance_reasons"`
	Segments               []SegmentResult   `json:"segments"`
	Trace                  []Trace           `json:"trace"`
	Issues                 []string          `json:"issues"`
}

type PunchAssignment struct {
	PunchID    string   `json:"punch_id"`
	BoundaryID string   `json:"boundary_id"`
	Role       string   `json:"role"`
	SegmentIDs []string `json:"segment_ids"`
}
