package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Recalculation and fingerprints load the same raw evidence by person/time.
// Index raw cards and approvals so a full month does not scan these tables
// once per employee. Version 6 has already shipped and stays immutable.
func ensureV7(ctx context.Context) error {
	indexes := []struct{ name, table, columns string }{
		{"biz_wecom_raw_user_time", "wecom_checkin_raw", "wecom_userid,checkin_time"},
		{"biz_att_leave_employee_time", "biz_hrm_attendance_leave", "employee_id,start_time,end_time"},
		{"biz_att_out_employee_time", "biz_hrm_attendance_out", "employee_id,start_time,end_time"},
		{"biz_att_trip_employee_time", "biz_hrm_attendance_trip", "employee_id,start_time,end_time"},
		{"biz_att_overtime_employee_time", "biz_hrm_attendance_overtime", "employee_id,start_time,end_time"},
	}
	statements := []string{}
	for _, index := range indexes {
		statements = append(statements, "CREATE INDEX "+index.name+" ON "+index.table+" ("+index.columns+")")
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	previous, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 7).One()
	if err != nil {
		return err
	}
	if len(previous) > 0 && previous["checksum"].String() != checksum {
		return fmt.Errorf("schema version 7 checksum changed")
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
			return fmt.Errorf("biz-suite schema version 7: %w", err)
		}
	}
	if len(previous) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 7, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	return err
}
