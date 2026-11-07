package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// EnsureSchema creates the business tables used by the biz-suite plugin. The
// statements deliberately use a small SQL subset supported by both MySQL and
// SQLite, because GoSuxin supports either database in development.
func EnsureSchema(ctx context.Context, extra ...string) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS biz_schema_version (version INTEGER NOT NULL PRIMARY KEY, checksum VARCHAR(64) NOT NULL, applied_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_employee (id VARCHAR(64) NOT NULL PRIMARY KEY, user_id BIGINT NULL UNIQUE, wecom_userid VARCHAR(128) NULL UNIQUE, employee_no VARCHAR(64) NULL UNIQUE, name VARCHAR(128) NOT NULL DEFAULT '', dept_id BIGINT NOT NULL DEFAULT 0, mobile VARCHAR(32) NOT NULL DEFAULT '', email VARCHAR(160) NOT NULL DEFAULT '', position VARCHAR(160) NOT NULL DEFAULT '', status VARCHAR(32) NOT NULL DEFAULT 'active', entry_date VARCHAR(32) NOT NULL DEFAULT '', quit_date VARCHAR(32) NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_group (id VARCHAR(64) NOT NULL PRIMARY KEY, name VARCHAR(128) NOT NULL, dept_ids TEXT NOT NULL, employee_ids TEXT NOT NULL, shifts TEXT NOT NULL, rule_json TEXT NOT NULL, status INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_holiday (id VARCHAR(64) NOT NULL PRIMARY KEY, holiday_date VARCHAR(16) NOT NULL, name VARCHAR(128) NOT NULL DEFAULT '', is_workday INTEGER NOT NULL DEFAULT 0, remark VARCHAR(255) NOT NULL DEFAULT '', created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE (holiday_date))`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_clock (id VARCHAR(64) NOT NULL PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, wecom_userid VARCHAR(128) NOT NULL DEFAULT '', clock_time DATETIME NOT NULL, clock_type VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL, source VARCHAR(32) NOT NULL, external_ref VARCHAR(192) NULL, latitude VARCHAR(64) NOT NULL DEFAULT '', longitude VARCHAR(64) NOT NULL DEFAULT '', address VARCHAR(255) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(source,external_ref))`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_leave (id VARCHAR(64) NOT NULL PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, source VARCHAR(32) NOT NULL, external_ref VARCHAR(192) NULL, approval_status VARCHAR(32) NOT NULL, leave_type VARCHAR(64) NOT NULL DEFAULT '', start_time DATETIME NOT NULL, end_time DATETIME NOT NULL, duration DECIMAL(12,2) NOT NULL, reason VARCHAR(500) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(source,external_ref))`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_out (id VARCHAR(64) NOT NULL PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, source VARCHAR(32) NOT NULL, external_ref VARCHAR(192) NULL, approval_status VARCHAR(32) NOT NULL, start_time DATETIME NOT NULL, end_time DATETIME NOT NULL, duration DECIMAL(12,2) NOT NULL, destination VARCHAR(255) NOT NULL DEFAULT '', reason VARCHAR(500) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(source,external_ref))`,
		`CREATE TABLE IF NOT EXISTS biz_migration_record (id VARCHAR(64) NOT NULL PRIMARY KEY, entity VARCHAR(64) NOT NULL, source_id VARCHAR(128) NOT NULL, target_id VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL, error_message VARCHAR(1000) NOT NULL, source_json TEXT NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(entity,source_id))`,
		`CREATE TABLE IF NOT EXISTS wecom_department (id VARCHAR(64) NOT NULL PRIMARY KEY, wecom_id BIGINT NOT NULL, parent_id BIGINT NOT NULL DEFAULT 0, name VARCHAR(160) NOT NULL, order_no INTEGER NOT NULL DEFAULT 0, host_dept_id BIGINT NOT NULL DEFAULT 0, raw_json TEXT NOT NULL, synced_at DATETIME NOT NULL, UNIQUE (wecom_id))`,
		`CREATE TABLE IF NOT EXISTS wecom_user (id VARCHAR(64) NOT NULL PRIMARY KEY, wecom_userid VARCHAR(128) NOT NULL, user_id BIGINT NOT NULL DEFAULT 0, employee_id VARCHAR(64) NOT NULL DEFAULT '', name VARCHAR(128) NOT NULL DEFAULT '', department_ids TEXT NOT NULL, position VARCHAR(160) NOT NULL DEFAULT '', mobile VARCHAR(32) NOT NULL DEFAULT '', email VARCHAR(160) NOT NULL DEFAULT '', status INTEGER NOT NULL DEFAULT 1, processing_error VARCHAR(1000) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, synced_at DATETIME NOT NULL, UNIQUE (wecom_userid))`,
		`CREATE TABLE IF NOT EXISTS wecom_checkin_raw (id VARCHAR(64) NOT NULL PRIMARY KEY, wecom_userid VARCHAR(128) NOT NULL, checkin_time DATETIME NOT NULL, checkin_type VARCHAR(64) NOT NULL DEFAULT '', exception_type VARCHAR(64) NOT NULL DEFAULT '', location VARCHAR(255) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, external_ref VARCHAR(192) NOT NULL, processing_error VARCHAR(1000) NOT NULL DEFAULT '', created_at DATETIME NOT NULL, UNIQUE (external_ref))`,
		`CREATE TABLE IF NOT EXISTS wecom_approval_raw (id VARCHAR(64) NOT NULL PRIMARY KEY, sp_no VARCHAR(128) NOT NULL, template_id VARCHAR(128) NOT NULL DEFAULT '', wecom_userid VARCHAR(128) NOT NULL DEFAULT '', approval_type VARCHAR(64) NOT NULL DEFAULT '', status VARCHAR(32) NOT NULL DEFAULT 'pending', apply_time DATETIME NULL, raw_json TEXT NOT NULL, detail_json TEXT NOT NULL, processing_error VARCHAR(1000) NOT NULL DEFAULT '', synced_at DATETIME NOT NULL, UNIQUE (sp_no))`,
		`CREATE TABLE IF NOT EXISTS wecom_approval_template (id VARCHAR(64) NOT NULL PRIMARY KEY, template_id VARCHAR(128) NOT NULL, approval_type VARCHAR(64) NOT NULL, mapping_json TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE (template_id))`,
		`CREATE TABLE IF NOT EXISTS wecom_sync_run (id VARCHAR(64) NOT NULL PRIMARY KEY, sync_type VARCHAR(32) NOT NULL, status VARCHAR(32) NOT NULL, started_at DATETIME NOT NULL, finished_at DATETIME NULL, success_count INTEGER NOT NULL DEFAULT 0, failure_count INTEGER NOT NULL DEFAULT 0, error_message VARCHAR(1000) NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS wecom_sync_cursor (sync_type VARCHAR(32) NOT NULL PRIMARY KEY, cursor_value VARCHAR(255) NOT NULL DEFAULT '', updated_at DATETIME NOT NULL)`,
	}
	statements = append(statements, extra...)
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	if _, err := core.DB().Exec(ctx, statements[0]); err != nil {
		return err
	}
	previous, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 1).One()
	if err != nil {
		return err
	}
	if len(previous) > 0 && previous["checksum"].String() != checksum {
		return fmt.Errorf("schema checksum changed: add a new migration before upgrading")
	}
	for _, statement := range statements {
		if _, err := core.DB().Exec(ctx, statement); err != nil {
			return fmt.Errorf("biz-suite schema: %w", err)
		}
	}
	if len(previous) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 1, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	if err != nil {
		return err
	}
	if err = ensureV2(ctx); err != nil {
		return err
	}
	if err = ensureV3(ctx); err != nil {
		return err
	}
	if err = ensureV4(ctx); err != nil {
		return err
	}
	if err = ensureV5(ctx); err != nil {
		return err
	}
	if err = ensureV6(ctx); err != nil {
		return err
	}
	return ensureV7(ctx)
}
