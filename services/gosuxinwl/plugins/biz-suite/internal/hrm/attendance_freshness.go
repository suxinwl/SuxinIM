package hrm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/approvalview"
	eng "github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/attendanceengine"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Fingerprint effective business evidence in a consistent read transaction.
// Synchronization timestamps, unrelated drafts and generated compensatory-leave
// ledger entries cannot change the fingerprint. JSON maps and unordered source
// evidence are canonicalized so repeated imports are harmless.
func attInputFingerprint(ctx context.Context, request AttBatchRequest) (string, error) {
	fingerprint, _, err := attInputFingerprints(ctx, request)
	return fingerprint, err
}

// Return both signatures from the same read snapshot. Source evidence excludes
// generated rosters, which the derivation worker is expected to replace, while
// retaining manual assignments and every input that can affect shift selection.
func attInputFingerprints(ctx context.Context, request AttBatchRequest) (string, string, error) {
	return attFingerprintSnapshot(ctx, request, true)
}

func attSourceFingerprint(ctx context.Context, request AttBatchRequest) (string, error) {
	_, source, err := attFingerprintSnapshot(ctx, request, false)
	return source, err
}

func attFingerprintSnapshot(ctx context.Context, request AttBatchRequest, includeGeneratedRules bool) (string, string, error) {
	var fingerprint, sourceFingerprint string
	err := core.DB().Transaction(ctx, func(txctx context.Context, _ gdb.TX) error {
		start, err := attDate(request.StartDate)
		if err != nil {
			return err
		}
		end, err := attDate(request.EndDate)
		if err != nil {
			return err
		}
		if end.Before(start) {
			return fmt.Errorf("计算日期范围无效")
		}
		from, to := start.AddDate(0, 0, -2).Format("2006-01-02 15:04:05"), end.AddDate(0, 0, 3).Format("2006-01-02 15:04:05")
		rosterFrom, rosterTo := start.AddDate(0, 0, -1).Format("2006-01-02"), end.AddDate(0, 0, 1).Format("2006-01-02")
		people, err := attEmployees(txctx)
		if err != nil {
			return err
		}
		// A newly published effective version must invalidate an older result
		// even before the background worker has replaced its derived roster.
		published, err := core.Model("biz_att_rule").Ctx(txctx).Where("status", "published").WhereLTE("effective_from", rosterTo).Fields("id,family_id,version,effective_from,effective_to,config_json,scope_json,conflicts_json").Order("id").All()
		if err != nil {
			return err
		}
		holidays, err := core.Model("biz_hrm_attendance_holiday").Ctx(txctx).WhereBetween("holiday_date", rosterFrom, rosterTo).Order("holiday_date").All()
		if err != nil {
			return err
		}
		ids := append([]string(nil), request.EmployeeIDs...)
		sort.Strings(ids)
		inputs := []g.Map{}
		sourceInputs := []g.Map{}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				return fmt.Errorf("计算人员范围重复，请重新试算")
			}
			seen[id] = true
			person, ok := people[id]
			if !ok {
				return fmt.Errorf("人员范围已变化，请重新试算")
			}
			employee, err := core.Model("biz_hrm_employee").Ctx(txctx).Where("id", id).One()
			if err != nil {
				return err
			}
			punches, err := attLoadPunches(txctx, id, from, to)
			if err != nil {
				return err
			}
			absences, err := attLoadAbsences(txctx, id, from, to)
			if err != nil {
				return err
			}
			sort.Slice(punches, func(i, j int) bool { return attJSON(punches[i]) < attJSON(punches[j]) })
			sort.Slice(absences, func(i, j int) bool { return attJSON(absences[i]) < attJSON(absences[j]) })
			rosterRows, err := core.Model("biz_att_roster").Ctx(txctx).Where("employee_id", id).WhereBetween("work_date", rosterFrom, rosterTo).Order("work_date,id").All()
			if err != nil {
				return err
			}
			rosters := []AttRoster{}
			manualRosters := []AttRoster{}
			rules := map[string]g.Map{}
			for _, row := range rosterRows {
				roster := attDecodeRoster(row)
				rosters = append(rosters, roster)
				if !attGeneratedRosterSource(roster.Source) {
					manualRosters = append(manualRosters, roster)
				}
				if includeGeneratedRules && roster.RuleID != "" && roster.Status == "confirmed" {
					if _, ok := rules[roster.RuleID]; !ok {
						rule, err := attLoadRule(txctx, roster.RuleID)
						if err != nil {
							return err
						}
						rules[rule.ID] = g.Map{"id": rule.ID, "version": rule.Version, "status": rule.Status, "config": rule.Config}
					}
				}
			}
			adjustmentRows, err := core.Model("biz_att_adjustment").Ctx(txctx).Where("employee_id", id).WhereBetween("work_date", rosterFrom, rosterTo).Order("work_date,created_at,id").All()
			if err != nil {
				return err
			}
			adjustments := []AttAdjustmentRequest{}
			for _, row := range adjustmentRows {
				var a AttAdjustmentRequest
				if err = json.Unmarshal([]byte(row["adjustment_json"].String()), &a); err != nil {
					return err
				}
				a.ID, a.EmployeeID, a.WorkDate = row["id"].String(), id, row["work_date"].String()
				adjustments = append(adjustments, a)
			}
			ledgerRows, err := core.Model("biz_att_ledger").Ctx(txctx).Where("employee_id", id).Order("id").All()
			if err != nil {
				return err
			}
			ledger := []g.Map{}
			for _, row := range ledgerRows {
				source := decodeLedgerSource(row)
				if source.Origin == "comp_leave" {
					continue
				}
				ledger = append(ledger, g.Map{"id": row["id"].String(), "entry_type": row["entry_type"].String(), "minutes": row["minutes"].Int64(), "effective_date": row["effective_date"].String(), "source_ref": row["source_ref"].String(), "reason": row["reason"].String(), "origin": source.Origin, "request": source.Request, "work_date": source.WorkDate, "result_id": source.ResultID, "reverses_entry_id": source.ReversesEntryID})
			}
			// Historical compensatory leave consumes balances used by current leave.
			// Keep the approvals, not their automatically generated debit/reversal rows.
			leaveRows, err := core.Model("biz_hrm_attendance_leave").Ctx(txctx).Where("employee_id", id).Order("id").All()
			if err != nil {
				return err
			}
			compLeaves := []g.Map{}
			for _, row := range leaveRows {
				name := strings.TrimSpace(row["leave_type"].String())
				if selected := approvalview.LeaveTypeLabel(row["raw_json"].String(), ""); selected != "" {
					name = selected
				}
				if !strings.Contains(name, "调休") {
					continue
				}
				compLeaves = append(compLeaves, g.Map{"id": row["id"].String(), "leave_type": name, "approval_status": row["approval_status"].String(), "start_time": row["start_time"].String(), "end_time": row["end_time"].String(), "duration": row["duration"].Float64(), "external_ref": row["external_ref"].String()})
			}
			inputs = append(inputs, g.Map{"employee": person, "wecom_userid": employee["wecom_userid"].String(), "employee_status": employee["status"].String(), "user_id": employee["user_id"].Int64(), "punches": punches, "absences": absences, "rosters": rosters, "rules": rules, "adjustments": adjustments, "manual_ledger": ledger, "comp_leaves": compLeaves})
			sourceInputs = append(sourceInputs, g.Map{"employee": person, "wecom_userid": employee["wecom_userid"].String(), "employee_status": employee["status"].String(), "user_id": employee["user_id"].Int64(), "punches": punches, "absences": absences, "rosters": manualRosters, "adjustments": adjustments, "manual_ledger": ledger, "comp_leaves": compLeaves})
		}
		bytes, err := json.Marshal(g.Map{"fingerprint_version": 2, "engine_version": eng.Version, "start_date": request.StartDate, "end_date": request.EndDate, "employees": inputs, "published_rules": published, "holidays": holidays})
		if err != nil {
			return err
		}
		sum := sha256.Sum256(bytes)
		fingerprint = hex.EncodeToString(sum[:])
		bytes, err = json.Marshal(g.Map{"fingerprint_version": 2, "engine_version": eng.Version, "start_date": request.StartDate, "end_date": request.EndDate, "employees": sourceInputs, "published_rules": published, "holidays": holidays})
		if err != nil {
			return err
		}
		sum = sha256.Sum256(bytes)
		sourceFingerprint = hex.EncodeToString(sum[:])
		return nil
	})
	return fingerprint, sourceFingerprint, err
}

func attFinishDerivationSnapshot(ctx context.Context, request AttBatchRequest, before string) (string, error) {
	fingerprint, after, err := attInputFingerprints(ctx, request)
	if err != nil {
		return "", err
	}
	if before != after {
		return "", fmt.Errorf("自动排班期间打卡、审批或人工调整等业务证据发生变化，请重新试算")
	}
	return fingerprint, nil
}

func attRequireFreshBatch(ctx context.Context, batchID string) error {
	batch, err := core.Model("biz_att_batch").Ctx(ctx).Where("id", batchID).One()
	if err != nil {
		return err
	}
	if len(batch) == 0 || batch["status"].String() != "completed" {
		return fmt.Errorf("计算批次不存在或未完成，请重新试算")
	}
	var request AttBatchRequest
	if json.Unmarshal([]byte(batch["request_json"].String()), &request) != nil || request.InputFingerprint == "" {
		return fmt.Errorf("该旧批次缺少输入校验记录，请重新试算复核")
	}
	people, err := attEmployees(ctx)
	if err != nil {
		return err
	}
	if len(people) != len(request.EmployeeIDs) {
		return fmt.Errorf("正式月报人员范围已变化，请重新试算复核")
	}
	current, err := attInputFingerprint(ctx, request)
	if err != nil {
		return err
	}
	if current != request.InputFingerprint {
		return fmt.Errorf("计算后业务证据或排班已变化，请重新试算复核")
	}
	return nil
}
