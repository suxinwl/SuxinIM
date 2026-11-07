package hrm

import (
	"context"
	"fmt"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Current directories and new business exclude archived employees. Historical
// attendance keeps its original employee association and is queried separately.
func currentEmployees(ctx context.Context, query *gdb.Model) (*gdb.Model, error) {
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil {
		return nil, err
	}
	query = query.WhereNot("status", "filtered")
	users := []string{}
	for user, excluded := range exclusions.Users {
		if excluded {
			users = append(users, user)
		}
	}
	if len(users) > 0 {
		query = query.Where("(wecom_userid IS NULL OR wecom_userid NOT IN (?))", users)
	}
	return query, nil
}

func manualEmployeeAllowed(ctx context.Context, id string) error {
	query, err := currentEmployees(ctx, core.Model("biz_hrm_employee").Ctx(ctx).Where("id", id))
	if err != nil {
		return err
	}
	ok, err := query.Exist()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("员工不存在或已被同步规则过滤；请先解除过滤，并由管理员恢复员工")
	}
	return nil
}

// The HTTP employee editor cannot reactivate an archived mapping by changing
// its source identifier. Sync and historical imports retain their own workflow.
func manualEmployeeChangeAllowed(ctx context.Context, in Employee) error {
	exclusions, err := core.LoadExplicitExclusions(ctx)
	if err != nil {
		return err
	}
	if exclusions.Users[in.WeComUserID] || in.Status == "filtered" {
		return fmt.Errorf("员工已被同步规则过滤；请先解除过滤，并由管理员恢复员工")
	}
	queries := []*gdb.Model{}
	if in.ID != "" {
		queries = append(queries, core.Model("biz_hrm_employee").Ctx(ctx).Where("id", in.ID))
	}
	if in.WeComUserID != "" {
		queries = append(queries, core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", in.WeComUserID))
	}
	if in.UserID > 0 {
		queries = append(queries, core.Model("biz_hrm_employee").Ctx(ctx).Where("user_id", in.UserID))
	}
	for _, query := range queries {
		rows, err := query.Fields("id,status,wecom_userid").All()
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row["status"].String() == "filtered" || exclusions.Users[row["wecom_userid"].String()] {
				return fmt.Errorf("员工已被同步规则过滤；请先解除过滤，并由管理员恢复员工")
			}
		}
	}
	return nil
}
