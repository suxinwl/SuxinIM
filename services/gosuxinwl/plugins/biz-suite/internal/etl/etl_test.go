package etl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/im"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
)

func setupDB(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	host, err := gdb.New(gdb.ConfigNode{Type: "sqlite", Link: fmt.Sprintf("sqlite::@file(%s)", filepath.ToSlash(filepath.Join(t.TempDir(), "etl.db"))), Prefix: "gf_", DeletedAt: "deletetime"})
	if err != nil {
		t.Fatal(err)
	}
	if err = core.Init(host); err != nil {
		t.Fatal(err)
	}
	business := core.DB()
	t.Cleanup(func() { _ = business.Close(ctx); _ = host.Close(ctx) })
	if err = migration.EnsureSchema(ctx, im.Schema()...); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{"CREATE TABLE gf_admin (id INTEGER PRIMARY KEY, username TEXT,mobile TEXT,status INTEGER NOT NULL,deletetime DATETIME)", "INSERT INTO gf_admin (id,username,mobile,status) VALUES (1,'alice','13800000001',0),(2,'bob','13800000002',0)", "CREATE TABLE gf_auth_dept (id INTEGER PRIMARY KEY)", "INSERT INTO gf_auth_dept(id) VALUES (10)"} {
		if _, err = core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	return ctx
}
func rows(t *testing.T, ctx context.Context, table string) int {
	t.Helper()
	n, err := core.Model(table).Ctx(ctx).Count()
	if err != nil {
		t.Fatal(err)
	}
	return n
}
func fixture() Document {
	return Document{Version: 1, Employees: []Employee{{SourceID: "employee-1", Identity: Identity{WeComUserID: "wx1", EmployeeNo: "EMP-1", HostUserID: 1}, Name: "Alice", HostDeptID: 10}}, Clocks: []Clock{{SourceID: "clock-1", EmployeeSourceID: "employee-1", ClockTime: "2024-01-02 09:00:00", ClockType: "in"}}, Leaves: []Absence{{SourceID: "leave-1", EmployeeSourceID: "employee-1", ApprovalStatus: "approved", StartTime: "2024-01-03 09:00:00", EndTime: "2024-01-03 10:00:00", Duration: 1}}, Outs: []Absence{{SourceID: "out-1", EmployeeSourceID: "employee-1", ApprovalStatus: "approved", StartTime: "2024-01-04 09:00:00", EndTime: "2024-01-04 10:00:00", Duration: 1, Destination: "office"}}}
}

func TestDryRunIsReadOnlyAndApplyIsIdempotent(t *testing.T) {
	ctx := setupDB(t)
	doc := fixture()
	preview, err := Run(ctx, doc, false)
	if err != nil || preview.Ready != 4 || preview.Failed != 0 {
		t.Fatal(preview, err)
	}
	for _, table := range []string{"biz_hrm_employee", "biz_hrm_attendance_clock", "biz_hrm_attendance_leave", "biz_hrm_attendance_out", "biz_migration_record"} {
		if rows(t, ctx, table) != 0 {
			t.Fatal("dry run wrote to", table)
		}
	}
	for i := 0; i < 2; i++ {
		report, e := Run(ctx, doc, true)
		if e != nil || report.Applied != 4 || report.Failed != 0 {
			t.Fatal(report, e)
		}
	}
	for _, table := range []string{"biz_hrm_employee", "biz_hrm_attendance_clock", "biz_hrm_attendance_leave", "biz_hrm_attendance_out"} {
		if rows(t, ctx, table) != 1 {
			t.Fatal("duplicate import", table)
		}
	}
	if rows(t, ctx, "biz_migration_record") != 4 || rows(t, ctx, "gf_admin") != 2 {
		t.Fatal("unexpected audit/account mutation")
	}
	clock, _ := core.Model("biz_hrm_attendance_clock").Ctx(ctx).One()
	if clock["id"].String() != preview.Results[1].TargetID || clock["source"].String() != "ruoyi" {
		t.Fatal(clock, preview.Results[1])
	}
}
func TestEmployeeMatchPriorityAndMappingPreservation(t *testing.T) {
	ctx := setupDB(t)
	id, err := hrm.UpsertEmployee(ctx, hrm.Employee{Name: "Local", UserID: 1, WeComUserID: "wx1", EmployeeNo: "EMP-1", DeptID: 10, Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	doc := Document{Version: 1, Employees: []Employee{{SourceID: "source-1", Identity: Identity{WeComUserID: "wx1"}, Name: "Alice"}}}
	report, err := Run(ctx, doc, true)
	if err != nil || report.Failed != 0 || report.Results[0].TargetID != id {
		t.Fatal(report, err)
	}
	row, _ := core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id).One()
	if row["user_id"].Int() != 1 || row["dept_id"].Int() != 10 || row["employee_no"].String() != "EMP-1" {
		t.Fatal(row)
	}
}
func TestAmbiguousIdentityPersistsFailedPayloadWithoutBlankAttendance(t *testing.T) {
	ctx := setupDB(t)
	for _, name := range []string{"first", "second"} {
		if _, e := hrm.UpsertEmployee(ctx, hrm.Employee{Name: name, Mobile: "duplicate"}); e != nil {
			t.Fatal(e)
		}
	}
	doc := Document{Version: 1, Employees: []Employee{{SourceID: "bad-employee", Identity: Identity{Mobile: "duplicate"}, Name: "Source Name"}}, Clocks: []Clock{{SourceID: "bad-clock", EmployeeSourceID: "bad-employee", ClockTime: "2024-01-02 09:00:00", ClockType: "in"}}}
	report, err := Run(ctx, doc, true)
	if err != nil || report.Failed != 2 {
		t.Fatal(report, err)
	}
	if rows(t, ctx, "biz_hrm_employee") != 2 || rows(t, ctx, "biz_hrm_attendance_clock") != 0 || rows(t, ctx, "biz_migration_record") != 2 {
		t.Fatal("invalid import mutation")
	}
	raw, _ := core.Model("biz_migration_record").Ctx(ctx).Where("source_id", "bad-employee").One()
	if !strings.Contains(raw["source_json"].String(), "Source Name") {
		t.Fatal("source payload lost", raw)
	}
}
func TestUsernameFallbackRequiresExplicitConfirmation(t *testing.T) {
	ctx := setupDB(t)
	doc := Document{Version: 1, IMUsers: []IMUser{{SourceID: "old-alice", Identity: Identity{Username: "alice"}}}}
	report, err := Run(ctx, doc, false)
	if err != nil || report.Failed != 1 {
		t.Fatal(report, err)
	}
	doc.IMUsers[0].ConfirmUsername = true
	report, err = Run(ctx, doc, true)
	if err != nil || report.Failed != 0 || report.Results[0].TargetID != "1" {
		t.Fatal(report, err)
	}
	if rows(t, ctx, "gf_admin") != 2 {
		t.Fatal("created host account")
	}
}
func TestHistoricalMessagesKeepTimestampAndDoNotDuplicate(t *testing.T) {
	ctx := setupDB(t)
	doc := Document{Version: 1, IMUsers: []IMUser{{SourceID: "old-1", Identity: Identity{HostUserID: 1}}, {SourceID: "old-2", Identity: Identity{HostUserID: 2}}}, Messages: []Message{
		{SourceID: "later", SenderSourceID: "old-1", ReceiverSourceID: "old-2", ConversationType: "private", MessageType: "text", Content: "second", SendTime: "2024-01-02 10:00:00"},
		{SourceID: "earlier", SenderSourceID: "old-1", ReceiverSourceID: "old-2", ConversationType: "private", MessageType: "text", Content: "first", SendTime: "2024-01-02 09:00:00"},
	}}
	preview, err := Run(ctx, doc, false)
	if err != nil || preview.Ready != 4 || preview.Failed != 0 || rows(t, ctx, "biz_im_message") != 0 {
		t.Fatal(preview, err)
	}
	for i := 0; i < 2; i++ {
		report, e := Run(ctx, doc, true)
		if e != nil || report.Failed != 0 || report.Applied != 4 {
			t.Fatal(report, e)
		}
	}
	messages, err := core.Model("biz_im_message").Ctx(ctx).OrderAsc("seq").All()
	if err != nil || len(messages) != 2 || messages[0]["content"].String() != "first" || !strings.HasPrefix(messages[0]["send_time"].String(), "2024-01-02 09:00:00") {
		t.Fatal(messages, err)
	}
}
func TestHistoricalMessageWithoutFriendshipPreservesDisabledAccountAndBlock(t *testing.T) {
	ctx := setupDB(t)
	for _, sql := range []string{
		"UPDATE gf_admin SET status=1 WHERE id=2",
		"INSERT INTO biz_im_friend(id,user_id,friend_user_id,display_name,status,created_at,updated_at) VALUES('block',2,1,'Historic note','blocked','2024-01-01','2024-01-01')",
	} {
		if _, err := core.DB().Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	doc := Document{Version: 1, IMUsers: []IMUser{{SourceID: "old-1", Identity: Identity{HostUserID: 1}}, {SourceID: "old-2", Identity: Identity{HostUserID: 2}}}, Messages: []Message{{SourceID: "msg-1", SenderSourceID: "old-1", ReceiverSourceID: "old-2", ConversationType: "private", MessageType: "text", Content: "private", SendTime: "2024-01-02 09:00:00"}}}
	for _, apply := range []bool{false, true} {
		report, err := Run(ctx, doc, apply)
		if err != nil || report.Failed != 0 {
			t.Fatal(report, err)
		}
		if !apply && rows(t, ctx, "biz_im_message") != 0 {
			t.Fatal("dry run wrote messages")
		}
	}
	// Previously resolved IDs must also remain usable while disabled.
	doc.IMUsers = nil
	if report, err := Run(ctx, doc, true); err != nil || report.Failed != 0 {
		t.Fatal(report, err)
	}
	if rows(t, ctx, "biz_im_message") != 1 || rows(t, ctx, "biz_im_friend") != 1 || rows(t, ctx, "biz_im_friend_request") != 0 {
		t.Fatal("historical import duplicated messages or provisioned social relationships")
	}
	account, _ := core.HostModel("admin").Ctx(ctx).Where("id", 2).One()
	block, _ := core.Model("biz_im_friend").Ctx(ctx).Where("id", "block").One()
	if account["status"].Int() != 1 || block["status"].String() != "blocked" || block["display_name"].String() != "Historic note" {
		t.Fatal("historical import changed account or contact settings")
	}
}

func TestHistoricalImportRejectsDeletedIdentity(t *testing.T) {
	ctx := setupDB(t)
	if _, err := core.DB().Exec(ctx, "UPDATE gf_admin SET deletetime='2026-01-01' WHERE id=2"); err != nil {
		t.Fatal(err)
	}
	doc := Document{Version: 1, IMUsers: []IMUser{{SourceID: "old-1", Identity: Identity{HostUserID: 1}}, {SourceID: "old-2", Identity: Identity{HostUserID: 2}}}, Messages: []Message{{SourceID: "bad-identity", SenderSourceID: "old-1", ReceiverSourceID: "old-2", ConversationType: "private", MessageType: "text", Content: "must remain quarantined", SendTime: "2024-01-02 09:00:00"}}}
	for _, apply := range []bool{false, true} {
		report, err := Run(ctx, doc, apply)
		if err != nil || report.Failed != 2 {
			t.Fatal(report, err)
		}
	}
	if rows(t, ctx, "biz_im_message") != 0 {
		t.Fatal("invalid historical identity produced a message")
	}
	trace, _ := core.Model("biz_migration_record").Ctx(ctx).Where("entity", "message").One()
	if trace["status"].String() != "failed" || !strings.Contains(trace["source_json"].String(), "quarantined") {
		t.Fatal(trace)
	}
}
func TestDecodeRejectsUnknownFieldsAndDuplicateSourceIDs(t *testing.T) {
	if _, err := Decode(strings.NewReader(`{"version":1,"oa":[]}`)); err == nil {
		t.Fatal("unknown OA field silently accepted")
	}
	if _, err := Decode(strings.NewReader(`{"version":1} {"version":1}`)); err == nil {
		t.Fatal("multiple documents accepted")
	}
	ctx := setupDB(t)
	doc := fixture()
	doc.Employees = append(doc.Employees, doc.Employees[0])
	if _, err := Run(ctx, doc, true); err == nil {
		t.Fatal("duplicate source IDs accepted")
	}
	if rows(t, ctx, "biz_hrm_employee") != 0 || rows(t, ctx, "biz_migration_record") != 0 {
		t.Fatal("partial duplicate document applied")
	}
}
