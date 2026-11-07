package wecom

import (
	"context"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecomprofile"
)

type exceptionIdentity struct {
	Name, EmployeeNo string
}

// Resolve identities by exact, unique directory/employee bindings. Display names
// are never join keys, and a numeric userid is never guessed to be an employee no.
func loadExceptionIdentities(ctx context.Context) (map[string]exceptionIdentity, error) {
	type employee struct{ id, uid, name, no string }
	type member struct{ employeeID, name, no string }
	rows, err := core.Model("biz_hrm_employee").Ctx(ctx).Fields("id,wecom_userid,name,employee_no").All()
	if err != nil {
		return nil, dbError("exception employee mapping")
	}
	employeesByID := map[string]employee{}
	employeesByUser := map[string][]employee{}
	uids := map[string]bool{}
	for _, row := range rows {
		e := employee{row["id"].String(), row["wecom_userid"].String(), strings.TrimSpace(row["name"].String()), strings.TrimSpace(row["employee_no"].String())}
		if e.id != "" {
			employeesByID[e.id] = e
		}
		if e.uid != "" {
			employeesByUser[e.uid] = append(employeesByUser[e.uid], e)
			uids[e.uid] = true
		}
	}
	rows, err = core.Model("wecom_user").Ctx(ctx).Fields("wecom_userid,employee_id,name,raw_json").All()
	if err != nil {
		return nil, dbError("exception directory mapping")
	}
	membersByUser := map[string][]member{}
	memberBindings := map[string]int{}
	for _, row := range rows {
		uid := row["wecom_userid"].String()
		if uid == "" {
			continue
		}
		m := member{row["employee_id"].String(), strings.TrimSpace(row["name"].String()), wecomprofile.EmployeeNoFromRaw(row["raw_json"].String())}
		membersByUser[uid] = append(membersByUser[uid], m)
		uids[uid] = true
		if m.employeeID != "" {
			memberBindings[m.employeeID]++
		}
	}
	out := map[string]exceptionIdentity{}
	for uid := range uids {
		var selected employee
		members, employees := membersByUser[uid], employeesByUser[uid]
		if len(employees) == 1 {
			selected = employees[0]
		} else if len(employees) == 0 && len(members) == 1 && memberBindings[members[0].employeeID] == 1 {
			candidate, exists := employeesByID[members[0].employeeID]
			if exists && (candidate.uid == "" || candidate.uid == uid) {
				selected = candidate
			}
		}
		identity := exceptionIdentity{Name: actualUserName(uid, selected.name), EmployeeNo: selected.no}
		if identity.Name == selected.id {
			identity.Name = ""
		}
		if len(members) == 1 {
			m := members[0]
			if identity.Name == "" {
				identity.Name = actualUserName(uid, m.name)
				if identity.Name == m.employeeID {
					identity.Name = ""
				}
			}
			if identity.EmployeeNo == "" {
				identity.EmployeeNo = m.no
			}
		}
		out[uid] = identity
	}
	return out, nil
}

func exceptionRecords(ctx context.Context) (g.Map, error) {
	identities, err := loadExceptionIdentities(ctx)
	if err != nil {
		return nil, err
	}
	const limit = 100
	totals := g.Map{}
	result := g.Map{"totals": totals, "limit": limit}
	for key, table := range map[string]string{"contacts": "wecom_user", "checkins": "wecom_checkin_raw", "approvals": "wecom_approval_raw"} {
		query := core.Model(table).Ctx(ctx).Where("processing_error != ?", "")
		total, err := query.Clone().Count()
		if err != nil {
			return nil, dbError("exception count")
		}
		totals[key] = total
		if key == "approvals" {
			configured, err := query.Clone().Where("template_id IN (?)", core.Model("wecom_approval_template").Ctx(ctx).Fields("template_id")).Count()
			if err != nil {
				return nil, dbError("exception template count")
			}
			result["approval_counts"] = g.Map{"configured": configured, "unmapped": total - configured}
			// Static SQL, with no request values. A large backlog of unmapped
			// templates must not hide failures of the configured business flows.
			query = query.Order(gdb.Raw(`CASE WHEN EXISTS (SELECT 1 FROM wecom_approval_template t WHERE t.template_id=wecom_approval_raw.template_id) THEN CASE WHEN processing_error='normalized approval failed HRM validation or persistence' THEN 0 ELSE 1 END ELSE 2 END`)).OrderDesc("synced_at").OrderAsc("id")
		} else if key == "checkins" {
			query = query.OrderDesc("checkin_time").OrderAsc("id")
		} else {
			query = query.OrderDesc("synced_at").OrderAsc("id")
		}
		rows, err := query.FieldsEx("raw_json,detail_json").Limit(limit).All()
		if err != nil {
			return nil, dbError("exception list")
		}
		items := make([]g.Map, 0, len(rows))
		for _, row := range rows {
			item := row.Map()
			identity := identities[row["wecom_userid"].String()]
			item["employee_name"], item["employee_no"] = identity.Name, identity.EmployeeNo
			items = append(items, item)
		}
		result[key] = items
	}
	return result, nil
}
