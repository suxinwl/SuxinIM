package core

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

// ExclusionRules contains only explicit exclusions. Missing upstream visibility
// and members outside an inclusion list are never evidence for removal.
type ExclusionRules struct {
	ExcludeDepartments     []int64  `json:"exclude_department_ids"`
	ExcludeDepartmentNames []string `json:"exclude_department_names"`
	ExcludeUsers           []string `json:"exclude_userids"`
	ExcludeUserNames       []string `json:"exclude_user_names"`
	IncludeSubdepartments  bool     `json:"include_subdepartments"`
}

type ExplicitExclusions struct {
	Departments map[int64]bool
	Users       map[string]bool
}

func LoadExplicitExclusions(ctx context.Context, transactions ...gdb.TX) (*ExplicitExclusions, error) {
	query := Model("wecom_settings").Ctx(ctx)
	if len(transactions) > 0 {
		query = query.TX(transactions[0])
	}
	row, err := query.Where("id", 1).One()
	if err != nil {
		return nil, err
	}
	rules := ExclusionRules{IncludeSubdepartments: true}
	if len(row) > 0 && json.Unmarshal([]byte(row["settings_json"].String()), &rules) != nil {
		return nil, fmt.Errorf("企业微信过滤配置无法读取")
	}
	return ExplicitExclusionsForRules(ctx, rules, transactions...)
}

// The optional transaction keeps candidate configuration and removal based on
// the same snapshot. No API request is made, and raw directory rows are retained.
func ExplicitExclusionsForRules(ctx context.Context, rules ExclusionRules, transactions ...gdb.TX) (*ExplicitExclusions, error) {
	query := func(table string) *gdb.Model {
		m := Model(table).Ctx(ctx)
		if len(transactions) > 0 {
			m = m.TX(transactions[0])
		}
		return m
	}
	out := &ExplicitExclusions{Departments: map[int64]bool{}, Users: map[string]bool{}}
	if len(rules.ExcludeDepartments)+len(rules.ExcludeDepartmentNames)+len(rules.ExcludeUsers)+len(rules.ExcludeUserNames) == 0 {
		return out, nil
	}
	rows, err := query("wecom_department").Fields("wecom_id,parent_id,name").All()
	if err != nil {
		return nil, err
	}
	parents, names := map[int64]int64{}, map[int64]string{}
	for _, row := range rows {
		id := row["wecom_id"].Int64()
		parents[id], names[id] = row["parent_id"].Int64(), strings.TrimSpace(row["name"].String())
	}
	departmentExcluded := func(id int64) bool {
		for seen := map[int64]bool{}; id > 0 && !seen[id]; id = parents[id] {
			if slices.Contains(rules.ExcludeDepartments, id) || (names[id] != "" && slices.Contains(rules.ExcludeDepartmentNames, names[id])) {
				return true
			}
			if !rules.IncludeSubdepartments {
				break
			}
			seen[id] = true
		}
		return false
	}
	for id := range parents {
		if departmentExcluded(id) {
			out.Departments[id] = true
		}
	}
	users, err := query("wecom_user").Fields("wecom_userid,name,department_ids").All()
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, row := range users {
		uid, name := row["wecom_userid"].String(), strings.TrimSpace(row["name"].String())
		if uid == "" {
			continue
		}
		known[uid] = true
		if slices.Contains(rules.ExcludeUsers, uid) || (name != "" && name != uid && slices.Contains(rules.ExcludeUserNames, name)) {
			out.Users[uid] = true
			continue
		}
		var ids []int64
		if json.Unmarshal([]byte(row["department_ids"].String()), &ids) == nil {
			for _, id := range ids {
				if departmentExcluded(id) {
					out.Users[uid] = true
					break
				}
			}
		}
	}
	// Old imports may have a unique employee mapping before the raw member row
	// exists. Only explicit user identity/name exclusions are provable there.
	employees, err := query("biz_hrm_employee").Fields("wecom_userid,name").All()
	if err != nil {
		return nil, err
	}
	for _, row := range employees {
		uid, name := row["wecom_userid"].String(), strings.TrimSpace(row["name"].String())
		if uid != "" && !known[uid] && (slices.Contains(rules.ExcludeUsers, uid) || (name != "" && name != uid && slices.Contains(rules.ExcludeUserNames, name))) {
			out.Users[uid] = true
		}
	}
	return out, nil
}
