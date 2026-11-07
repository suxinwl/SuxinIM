package hrm

import (
	"context"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
	"path/filepath"
	"testing"
	"time"
)

func setup(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	db, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Name: filepath.Join(t.TempDir(), "hrm.db"), Prefix: "gf_", DeletedAt: "deletetime"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Init(db); err != nil {
		t.Fatal(err)
	}
	business := core.DB()
	t.Cleanup(func() { business.Close(ctx); db.Close(ctx) })
	for _, sql := range []string{
		`CREATE TABLE gf_admin(id INTEGER PRIMARY KEY,name TEXT,username TEXT,nickname TEXT,mobile TEXT,status INTEGER DEFAULT 0,dept_id INTEGER DEFAULT 0,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_dept(id INTEGER PRIMARY KEY,name TEXT,pid INTEGER,status INTEGER DEFAULT 0,deletetime DATETIME)`,
		`INSERT INTO gf_admin(id,name,username,mobile,status,dept_id) VALUES(1,'Alice','alice','13800000000',0,10)`,
		`INSERT INTO gf_auth_dept(id,name,pid) VALUES(10,'Dev',0)`,
		`CREATE TABLE gf_auth_role_access(uid INTEGER,role_id INTEGER)`,
		`CREATE TABLE gf_auth_role(id INTEGER PRIMARY KEY,rules TEXT,btns TEXT,status INTEGER DEFAULT 0,deletetime DATETIME)`,
		`CREATE TABLE gf_auth_rule(id INTEGER PRIMARY KEY,permission TEXT,type INTEGER DEFAULT 2,status INTEGER DEFAULT 0,deletetime DATETIME)`,
		`INSERT INTO gf_auth_role_access VALUES(1,1)`,
		`INSERT INTO gf_auth_role(id,rules,btns) VALUES(1,'','1,2,3')`,
		`INSERT INTO gf_auth_rule(id,permission) VALUES(1,'biz-suite.hrm.read'),(2,'biz-suite.hrm.write'),(3,'biz-suite.hrm.manage')`,
	} {
		if _, err = db.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	if err = migration.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err = migration.EnsureSchema(ctx); err != nil {
		t.Fatal("migration is not idempotent", err)
	}
	return ctx
}
func TestUpsertsPreserveMappingsAndRejectInvalidRecords(t *testing.T) {
	ctx := setup(t)
	id, err := UpsertEmployee(ctx, Employee{Name: "Alice", UserID: 1, DeptID: 10, WeComUserID: "alice", EmployeeNo: "E001"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := UpsertEmployee(ctx, Employee{Name: "Alice updated", WeComUserID: "alice"})
	if err != nil || again != id {
		t.Fatalf("repeat %s %v", again, err)
	}
	row, err := core.Model("biz_hrm_employee").Where("id", id).One()
	if err != nil || row["user_id"].Int() != 1 || row["dept_id"].Int() != 10 || row["employee_no"].String() != "E001" {
		t.Fatalf("lost mapping: %v %v", row, err)
	}
	for _, name := range []string{"Bob", "Carol"} {
		if _, err = UpsertEmployee(ctx, Employee{Name: name}); err != nil {
			t.Fatal("empty optional identifiers must not collide", err)
		}
	}
	if _, err = UpsertEmployee(ctx, Employee{Name: "Collision", UserID: 1}); err == nil {
		t.Fatal("duplicate host mapping accepted")
	}
	clock := Clock{EmployeeID: id, ClockType: "in", ClockTime: "2026-01-05T01:00:00Z", Source: "wecom", ExternalRef: "clock1"}
	a, err := UpsertClock(ctx, clock)
	if err != nil {
		t.Fatal(err)
	}
	b, err := UpsertClock(ctx, clock)
	if err != nil || a != b {
		t.Fatal("duplicate clock", err)
	}
	clock.Source = "ruoyi"
	if _, err = UpsertClock(ctx, clock); err != nil {
		t.Fatal("references from different sources collide", err)
	}
	clock.Source = "manual"
	clock.ExternalRef = ""
	for i := 0; i < 2; i++ {
		if _, err = UpsertClock(ctx, clock); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := core.Model("biz_hrm_attendance_clock").Count()
	if n != 4 {
		t.Fatalf("clock count %d", n)
	}
	clock.ClockTime = "nonsense"
	if _, err = UpsertClock(ctx, clock); err == nil {
		t.Fatal("invalid time accepted")
	}
	clock.ClockTime = "2026-01-05 09:00:00"
	clock.EmployeeID = "missing"
	if _, err = UpsertClock(ctx, clock); err == nil {
		t.Fatal("orphan clock accepted")
	}
}
func tm(s string) time.Time {
	v, e := core.ParseTime(s)
	if e != nil {
		panic(e)
	}
	return v
}
func TestAttendanceRules(t *testing.T) {
	date := tm("2026-01-05")
	at := tm("2026-01-07")
	shift := Shift{Start: "09:00", End: "18:00", Weekdays: []int{1, 2, 3, 4, 5}, BreakStart: "12:00", BreakEnd: "13:00"}
	clocks := []Clock{{ClockTime: "2026-01-05 09:07:00", ClockType: "in"}, {ClockTime: "2026-01-05 17:55:00", ClockType: "out"}}
	d := calculateDay(date, []Shift{shift}, Rule{LateGraceMinutes: 5, EarlyGraceMinutes: 5}, clocks, nil, nil, nil, at)
	if d.ExpectedHours != 8 || d.LateCount != 1 || d.EarlyCount != 0 || d.MissingCount != 0 || !d.Worked {
		t.Fatalf("wrong day %+v", d)
	}
	leaves := []span{{tm("2026-01-05 09:00:00"), tm("2026-01-05 13:00:00")}, {tm("2026-01-05 10:00:00"), tm("2026-01-05 14:00:00")}}
	outs := []span{{tm("2026-01-05 13:00:00"), tm("2026-01-05 18:00:00")}}
	d = calculateDay(date, []Shift{shift}, Rule{}, nil, leaves, outs, nil, at)
	if d.LeaveHours != 4 || d.OutHours != 4 || d.MissingCount != 0 || d.Absent {
		t.Fatalf("overlap or break counted twice %+v", d)
	}
	d = calculateDay(date, []Shift{shift}, Rule{}, nil, nil, nil, nil, date)
	if d.State != "pending" || d.MissingCount != 0 || d.Absent {
		t.Fatalf("future marked absent %+v", d)
	}
	holiday := false
	d = calculateDay(date, []Shift{shift}, Rule{}, nil, nil, nil, &holiday, at)
	if d.ExpectedHours != 0 {
		t.Fatal("holiday is not rest")
	}
	d = calculateDay(date, []Shift{shift}, Rule{}, nil, nil, nil, nil, at)
	if !d.Absent || d.MissingCount != 2 {
		t.Fatalf("missing %+v", d)
	}
	night := Shift{Start: "22:00", End: "06:00", Weekdays: []int{1}}
	d = calculateDay(date, []Shift{night}, Rule{}, []Clock{{ClockType: "in", ClockTime: "2026-01-05 22:00:00"}, {ClockType: "out", ClockTime: "2026-01-06 06:00:00"}}, nil, nil, nil, at)
	if d.ExpectedHours != 8 || !d.Worked || d.LateCount+d.EarlyCount+d.MissingCount != 0 {
		t.Fatalf("overnight %+v", d)
	}
	split := []Shift{{Start: "09:00", End: "12:00", Weekdays: []int{1}}, {Start: "13:00", End: "18:00", Weekdays: []int{1}}}
	d = calculateDay(date, split, Rule{}, []Clock{{ClockType: "in", ClockTime: "2026-01-05 09:00:00"}, {ClockType: "out", ClockTime: "2026-01-05 12:00:00"}}, nil, nil, nil, at)
	if d.MissingCount != 2 {
		t.Fatalf("punch reused across shifts %+v", d)
	}
	workday := true
	d = calculateDay(tm("2026-01-10"), []Shift{{Start: "09:00", End: "18:00", Weekdays: []int{1}}, {Start: "10:00", End: "19:00", Weekdays: []int{2}}}, Rule{}, nil, nil, nil, &workday, at.AddDate(0, 0, 7))
	if d.State != "ambiguous_schedule" || d.ExpectedHours != 0 {
		t.Fatalf("ambiguous makeup silently double counted %+v", d)
	}
}
func TestApprovalRevocationRecalculatesMonth(t *testing.T) {
	ctx := setup(t)
	id, err := UpsertEmployee(ctx, Employee{Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = core.Model("biz_hrm_attendance_group").Data(g.Map{"id": "g1", "name": "Day", "dept_ids": "[]", "employee_ids": "[\"" + id + "\"]", "shifts": "[{\"start\":\"09:00\",\"end\":\"18:00\",\"weekdays\":[1],\"break_start\":\"12:00\",\"break_end\":\"13:00\"}]", "rule_json": "{}", "status": 1, "created_at": core.Now(), "updated_at": core.Now()}).Insert()
	if err != nil {
		t.Fatal(err)
	}
	leave := Leave{EmployeeID: id, Source: "wecom", ExternalRef: "sp1", ApprovalStatus: "approved", StartTime: "2026-01-05 09:00:00", EndTime: "2026-01-05 18:00:00", Duration: 8}
	a, err := UpsertLeave(ctx, leave)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Month(ctx, "2026-01", id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].LeaveHours != 8 {
		t.Fatalf("approved missing %+v", result)
	}
	leave.ApprovalStatus = "canceled"
	b, err := UpsertLeave(ctx, leave)
	if err != nil || a != b {
		t.Fatal("revocation created duplicate", err)
	}
	result, err = Month(ctx, "2026-01", id)
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].LeaveHours != 0 || result.Items[0].MissingCount != 8 {
		t.Fatalf("revocation not applied %+v", result)
	}
}

func TestParseTime(t *testing.T) {
	if got := parseTime("2026-10-28T12:30:00Z"); got != "2026-10-28 20:30:00" {
		t.Fatalf("got %q", got)
	}
	if got := parseTime(""); got != "" {
		t.Fatalf("invalid empty time %q", got)
	}
}
