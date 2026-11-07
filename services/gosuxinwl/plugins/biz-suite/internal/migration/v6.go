package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// A full historical rebuild creates one result per employee/day. These lookup
// indexes keep the original clock page responsive when a punch is unassigned
// because its rule is still awaiting configuration. No business rows change.
func ensureV6(ctx context.Context) error {
	indexes := []struct{ name, table, columns string }{
		{"biz_att_result_employee_day", "biz_att_result", "employee_id,work_date,batch_id"},
		{"biz_clock_employee_time", "biz_hrm_attendance_clock", "employee_id,clock_time"},
		{"biz_clock_time_id", "biz_hrm_attendance_clock", "clock_time,id"},
		{"biz_att_clock_result", "biz_att_clock_state", "result_id"},
	}
	statements := []string{}
	for _, index := range indexes {
		statements = append(statements, "CREATE INDEX "+index.name+" ON "+index.table+" ("+index.columns+")")
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	previous, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 6).One()
	if err != nil {
		return err
	}
	if len(previous) > 0 && previous["checksum"].String() != checksum {
		return fmt.Errorf("schema version 6 checksum changed")
	}
	for i, index := range indexes {
		query := "SELECT name FROM sqlite_master WHERE type='index' AND tbl_name=? AND name=?"
		if core.DB().GetConfig().Type == "mysql" {
			query = "SELECT index_name FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND index_name=? LIMIT 1"
		}
		found, err := core.DB().GetOne(ctx, query, index.table, index.name)
		if err != nil {
			return err
		}
		if len(found) > 0 {
			continue
		}
		if _, err = core.DB().Exec(ctx, statements[i]); err != nil {
			return fmt.Errorf("biz-suite schema version 6: %w", err)
		}
	}
	if len(previous) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 6, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	return err
}
