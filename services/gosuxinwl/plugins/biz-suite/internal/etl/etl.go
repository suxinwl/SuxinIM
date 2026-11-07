package etl

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/im"
)

func Decode(reader io.Reader) (Document, error) {
	var d Document
	decoder := json.NewDecoder(io.LimitReader(reader, 128<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return d, errors.New("invalid versioned migration JSON or unknown field")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return d, errors.New("migration input must contain exactly one JSON document")
	}
	if d.Version != 1 {
		return d, errors.New("migration version must be 1")
	}
	return d, nil
}
func idFor(kind, id string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte("ruoyi:"+kind+":"+id)))
}
func attendanceID(kind, id string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(kind+":ruoyi:"+id)))
}
func rawJSON(v any) string   { b, _ := json.Marshal(v); return string(b) }
func validID(id string) bool { return strings.TrimSpace(id) != "" && len(id) <= 120 }
func databaseError() error {
	return errors.New("migration database operation failed; verify plugin schema and database access")
}

type runner struct {
	ctx                          context.Context
	apply                        bool
	report                       Report
	employees                    map[string]hrm.Employee
	users                        map[string]int64
	failedEmployees, failedUsers map[string]bool
}

// Run returns one outcome per source row. Apply uses per-row transactions:
// successful imports and their trace rows commit together; failed rows retain
// their source JSON for correction/replay. It does not provision friendships.
func Run(ctx context.Context, d Document, apply bool) (Report, error) {
	r := &runner{ctx: ctx, apply: apply, report: Report{DryRun: !apply, Results: []Result{}}, employees: map[string]hrm.Employee{}, users: map[string]int64{}, failedEmployees: map[string]bool{}, failedUsers: map[string]bool{}}
	if d.Version != 1 {
		return r.report, errors.New("migration version must be 1")
	}
	// Reject duplicate identities before the first mutation, rather than quietly
	// letting the second source row overwrite the first row's audit payload.
	groups := map[string][]string{}
	for _, v := range d.Employees {
		groups["employee"] = append(groups["employee"], v.SourceID)
	}
	for _, v := range d.Clocks {
		groups["clock"] = append(groups["clock"], v.SourceID)
	}
	for _, v := range d.Leaves {
		groups["leave"] = append(groups["leave"], v.SourceID)
	}
	for _, v := range d.Outs {
		groups["out"] = append(groups["out"], v.SourceID)
	}
	for _, v := range d.IMUsers {
		groups["im_user"] = append(groups["im_user"], v.SourceID)
	}
	for _, v := range d.Messages {
		groups["message"] = append(groups["message"], v.SourceID)
	}
	for kind, ids := range groups {
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				return r.report, fmt.Errorf("duplicate source_id in %s export", kind)
			}
			seen[id] = true
		}
	}
	if _, err := core.Model("biz_migration_record").Ctx(ctx).Limit(1).One(); err != nil {
		return r.report, errors.New("biz-suite schema is not available; install the plugin before running ETL")
	}
	for _, v := range d.Employees {
		employee, err := r.employee(v)
		result, e := r.record("employee", v.SourceID, v, employee.ID, err, func(txctx context.Context) (string, error) { return hrm.UpsertEmployee(txctx, employee) })
		if e != nil {
			return r.report, e
		}
		if result.Error == "" {
			employee.ID = result.TargetID
			r.employees[v.SourceID] = employee
		} else {
			r.failedEmployees[v.SourceID] = true
		}
	}
	for _, v := range d.Clocks {
		employee, err := r.employeeRef(v.EmployeeSourceID)
		if err == nil {
			err = validateClock(v)
		}
		_, e := r.record("clock", v.SourceID, v, attendanceID("clock", v.SourceID), err, func(txctx context.Context) (string, error) {
			return hrm.UpsertClock(txctx, hrm.Clock{EmployeeID: employee.ID, WeComUserID: employee.WeComUserID, Source: "ruoyi", ExternalRef: v.SourceID, ClockTime: v.ClockTime, ClockType: v.ClockType, Status: defaultString(v.Status, "normal"), Address: v.Address, Latitude: v.Latitude, Longitude: v.Longitude, RawJSON: rawJSON(v)})
		})
		if e != nil {
			return r.report, e
		}
	}
	for _, kind := range []string{"leave", "out"} {
		list := d.Leaves
		if kind == "out" {
			list = d.Outs
		}
		for _, v := range list {
			employee, err := r.employeeRef(v.EmployeeSourceID)
			if err == nil {
				err = validateAbsence(v)
			}
			_, e := r.record(kind, v.SourceID, v, attendanceID(kind, v.SourceID), err, func(txctx context.Context) (string, error) {
				if kind == "leave" {
					return hrm.UpsertLeave(txctx, hrm.Leave{EmployeeID: employee.ID, Source: "ruoyi", ExternalRef: v.SourceID, ApprovalStatus: v.ApprovalStatus, StartTime: v.StartTime, EndTime: v.EndTime, Duration: v.Duration, Reason: v.Reason, LeaveType: v.LeaveType, RawJSON: rawJSON(v)})
				}
				return hrm.UpsertOut(txctx, hrm.Out{EmployeeID: employee.ID, Source: "ruoyi", ExternalRef: v.SourceID, ApprovalStatus: v.ApprovalStatus, StartTime: v.StartTime, EndTime: v.EndTime, Duration: v.Duration, Reason: v.Reason, Destination: v.Destination, RawJSON: rawJSON(v)})
			})
			if e != nil {
				return r.report, e
			}
		}
	}
	for _, v := range d.IMUsers {
		user, err := r.resolveHistoricalUser(v.Identity)
		result, e := r.record("im_user", v.SourceID, v, strconv.FormatInt(user, 10), err, func(context.Context) (string, error) { return strconv.FormatInt(user, 10), nil })
		if e != nil {
			return r.report, e
		}
		if result.Error == "" {
			r.users[v.SourceID] = user
		} else {
			r.failedUsers[v.SourceID] = true
		}
	}
	messages := append([]Message(nil), d.Messages...)
	sort.SliceStable(messages, func(i, j int) bool {
		a, _ := core.ParseTime(messages[i].SendTime)
		b, _ := core.ParseTime(messages[j].SendTime)
		if a.Equal(b) {
			return messages[i].SourceID < messages[j].SourceID
		}
		return a.Before(b)
	})
	for _, v := range messages {
		sender, err := r.userRef(v.SenderSourceID)
		var receiver int64
		if err == nil && v.ConversationType == "private" {
			receiver, err = r.userRef(v.ReceiverSourceID)
		}
		in := im.MessageInput{ConversationType: v.ConversationType, ConversationID: v.ConversationID, ReceiverID: receiver, ClientMessageID: "ruoyi:" + v.SourceID, MessageType: v.MessageType, Content: v.Content}
		if err == nil {
			err = im.ValidateImport(ctx, sender, in, v.SendTime)
			if err != nil {
				err = errors.New("message validation failed: check timestamp, type, content, mapped participants, group membership and existing source ID")
			}
		}
		_, e := r.record("message", v.SourceID, v, "", err, func(txctx context.Context) (string, error) {
			out, e := im.ImportHistoricalMessage(txctx, sender, in, v.SendTime)
			if e != nil {
				return "", e
			}
			return fmt.Sprint(out["id"]), nil
		})
		if e != nil {
			return r.report, e
		}
	}
	return r.report, nil
}

func (r *runner) record(kind, source string, raw any, target string, validation error, write func(context.Context) (string, error)) (Result, error) {
	result := Result{Entity: kind, SourceID: source, TargetID: target, Status: "ready"}
	if !validID(source) {
		validation = errors.New("source_id must contain 1 to 120 bytes")
	}
	if err := r.ctx.Err(); err != nil {
		return result, err
	}
	if validation == nil && r.apply {
		err := core.DB().Transaction(r.ctx, func(ctx context.Context, _ gdb.TX) error {
			var err error
			result.TargetID, err = write(ctx)
			if err != nil {
				return errors.New("target validation or persistence failed; correct mappings and retry")
			}
			result.Status = "success"
			return saveTrace(ctx, result, rawJSON(raw))
		})
		if err != nil {
			validation = errors.New("target validation or persistence failed; correct mappings and retry")
		}
	}
	if validation != nil {
		result.Status = "failed"
		result.Error = validation.Error()
		result.TargetID = ""
		r.report.Failed++
		if r.apply {
			if err := saveTrace(r.ctx, result, rawJSON(raw)); err != nil {
				return result, err
			}
		}
	} else if r.apply {
		r.report.Applied++
	} else {
		r.report.Ready++
	}
	r.report.Results = append(r.report.Results, result)
	return result, nil
}
func saveTrace(ctx context.Context, result Result, source string) error {
	if !validID(result.SourceID) {
		result.SourceID = "invalid:" + idFor("source", result.SourceID)
	}
	data := g.Map{"id": idFor("trace:"+result.Entity, result.SourceID), "entity": result.Entity, "source_id": result.SourceID, "target_id": result.TargetID, "status": result.Status, "error_message": result.Error, "source_json": source, "updated_at": core.Now()}
	row, err := core.Model("biz_migration_record").Ctx(ctx).Where(g.Map{"entity": result.Entity, "source_id": result.SourceID}).One()
	if err != nil {
		return databaseError()
	}
	if len(row) > 0 {
		delete(data, "id")
		_, err = core.Model("biz_migration_record").Ctx(ctx).Where("id", row["id"].String()).Update(data)
	} else {
		_, err = core.Model("biz_migration_record").Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return databaseError()
	}
	return nil
}

func (r *runner) employeeRef(source string) (hrm.Employee, error) {
	if r.failedEmployees[source] {
		return hrm.Employee{}, errors.New("source employee failed migration")
	}
	if v, ok := r.employees[source]; ok {
		return v, nil
	}
	row, err := core.Model("biz_migration_record").Ctx(r.ctx).Where(g.Map{"entity": "employee", "source_id": source, "status": "success"}).One()
	if err != nil {
		return hrm.Employee{}, databaseError()
	}
	if len(row) == 0 {
		return hrm.Employee{}, errors.New("employee_source_id is not mapped; import the employee first")
	}
	target, err := core.Model("biz_hrm_employee").Ctx(r.ctx).Where("id", row["target_id"].String()).One()
	if err != nil {
		return hrm.Employee{}, databaseError()
	}
	if len(target) == 0 {
		return hrm.Employee{}, errors.New("mapped employee no longer exists")
	}
	var v hrm.Employee
	if target.Struct(&v) != nil {
		return v, databaseError()
	}
	return v, nil
}
func (r *runner) userRef(source string) (int64, error) {
	if r.failedUsers[source] {
		return 0, errors.New("source IM user failed mapping")
	}
	if id, ok := r.users[source]; ok {
		return id, nil
	}
	row, err := core.Model("biz_migration_record").Ctx(r.ctx).Where(g.Map{"entity": "im_user", "source_id": source, "status": "success"}).One()
	if err != nil {
		return 0, databaseError()
	}
	if len(row) == 0 {
		return 0, errors.New("IM user source_id is not mapped")
	}
	id := row["target_id"].Int64()
	ok, err := core.HostModel("admin").Ctx(r.ctx).Where("id", id).Exist()
	if err != nil {
		return 0, databaseError()
	}
	if !ok {
		return 0, errors.New("mapped host user no longer exists")
	}
	return id, nil
}

func validateClock(v Clock) error {
	if _, err := core.ParseTime(v.ClockTime); err != nil {
		return errors.New("invalid clock_time")
	}
	if v.ClockType != "in" && v.ClockType != "out" && v.ClockType != "outside" {
		return errors.New("clock_type must be in/out/outside")
	}
	if v.Status != "" && v.Status != "normal" && v.Status != "late" && v.Status != "early" && v.Status != "missing" && v.Status != "abnormal" {
		return errors.New("invalid clock status")
	}
	return nil
}
func validateAbsence(v Absence) error {
	start, e := core.ParseTime(v.StartTime)
	if e != nil {
		return errors.New("invalid absence start_time")
	}
	end, e := core.ParseTime(v.EndTime)
	if e != nil || !end.After(start) {
		return errors.New("invalid absence end_time")
	}
	if math.IsNaN(v.Duration) || math.IsInf(v.Duration, 0) || v.Duration < 0 || v.Duration > end.Sub(start).Hours()+0.01 {
		return errors.New("duration must be hours within the absence interval")
	}
	if v.ApprovalStatus != "pending" && v.ApprovalStatus != "approved" && v.ApprovalStatus != "rejected" && v.ApprovalStatus != "canceled" {
		return errors.New("invalid approval_status")
	}
	return nil
}
func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func validateDates(entry, quit string) error {
	for _, v := range []string{entry, quit} {
		if v != "" {
			if _, e := time.Parse("2006-01-02", v); e != nil {
				return errors.New("employee dates must be YYYY-MM-DD")
			}
		}
	}
	if entry != "" && quit != "" && quit < entry {
		return errors.New("quit_date precedes entry_date")
	}
	return nil
}
