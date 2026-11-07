package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"strings"
)

// Rules, evidence and published results are separate, additive records. Never
// alter a previous migration checksum or replace an upstream attendance row.
func ensureV5(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS wecom_attendance_outbox (id VARCHAR(64) PRIMARY KEY, start_time DATETIME NOT NULL, end_time DATETIME NOT NULL, employee_ids LONGTEXT NOT NULL, status VARCHAR(32) NOT NULL DEFAULT 'pending', error_message VARCHAR(2000) NOT NULL DEFAULT '', created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_rule (id VARCHAR(64) PRIMARY KEY, family_id VARCHAR(64) NOT NULL, version INTEGER NOT NULL, name VARCHAR(160) NOT NULL, status VARCHAR(32) NOT NULL, effective_from VARCHAR(10) NOT NULL, effective_to VARCHAR(10) NOT NULL DEFAULT '', config_json LONGTEXT NOT NULL, scope_json LONGTEXT NOT NULL, conflicts_json LONGTEXT NOT NULL, source_json LONGTEXT NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(family_id,version))`,
		`CREATE TABLE IF NOT EXISTS biz_att_roster (id VARCHAR(64) PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, work_date VARCHAR(10) NOT NULL, rule_id VARCHAR(64) NOT NULL, shift_id VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL, source VARCHAR(32) NOT NULL, roster_json LONGTEXT NOT NULL, updated_by BIGINT NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(employee_id,work_date))`,
		`CREATE TABLE IF NOT EXISTS biz_att_batch (id VARCHAR(64) PRIMARY KEY, start_date VARCHAR(10) NOT NULL, end_date VARCHAR(10) NOT NULL, status VARCHAR(32) NOT NULL, request_json LONGTEXT NOT NULL, employee_count INTEGER NOT NULL DEFAULT 0, total_count INTEGER NOT NULL DEFAULT 0, completed_count INTEGER NOT NULL DEFAULT 0, error_message VARCHAR(2000) NOT NULL DEFAULT '', created_by BIGINT NOT NULL, created_at DATETIME NOT NULL, finished_at DATETIME NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_result (id VARCHAR(64) PRIMARY KEY, batch_id VARCHAR(64) NOT NULL, employee_id VARCHAR(64) NOT NULL, work_date VARCHAR(10) NOT NULL, rule_id VARCHAR(64) NOT NULL, status VARCHAR(32) NOT NULL, employee_json LONGTEXT NOT NULL, result_json LONGTEXT NOT NULL, input_json LONGTEXT NOT NULL, created_at DATETIME NOT NULL, UNIQUE(batch_id,employee_id,work_date))`,
		`CREATE TABLE IF NOT EXISTS biz_att_month (month VARCHAR(7) PRIMARY KEY, status VARCHAR(32) NOT NULL, batch_id VARCHAR(64) NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 0, change_pending INTEGER NOT NULL DEFAULT 0, updated_by BIGINT NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_publication (id VARCHAR(64) PRIMARY KEY, month VARCHAR(7) NOT NULL, revision INTEGER NOT NULL, batch_id VARCHAR(64) NOT NULL, action VARCHAR(32) NOT NULL, reason VARCHAR(1000) NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL, UNIQUE(month,revision))`,
		`CREATE TABLE IF NOT EXISTS biz_att_adjustment (id VARCHAR(64) PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, work_date VARCHAR(10) NOT NULL, adjustment_json LONGTEXT NOT NULL, reason VARCHAR(1000) NOT NULL, evidence VARCHAR(2000) NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_ledger (id VARCHAR(64) PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, entry_type VARCHAR(32) NOT NULL, minutes INTEGER NOT NULL, effective_date VARCHAR(10) NOT NULL, source_ref VARCHAR(192) NOT NULL UNIQUE, source_json LONGTEXT NOT NULL, reason VARCHAR(1000) NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_import (id VARCHAR(64) PRIMARY KEY, kind VARCHAR(32) NOT NULL, filename VARCHAR(255) NOT NULL, content_hash VARCHAR(64) NOT NULL UNIQUE, month VARCHAR(7) NOT NULL DEFAULT '', preview_json LONGTEXT NOT NULL, mappings_json LONGTEXT NOT NULL, status VARCHAR(32) NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_audit (id VARCHAR(64) PRIMARY KEY, entity VARCHAR(32) NOT NULL, entity_id VARCHAR(192) NOT NULL, action VARCHAR(32) NOT NULL, before_json LONGTEXT NOT NULL, after_json LONGTEXT NOT NULL, reason VARCHAR(1000) NOT NULL, created_by BIGINT NOT NULL, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_dirty (id VARCHAR(64) PRIMARY KEY, start_date VARCHAR(10) NOT NULL, end_date VARCHAR(10) NOT NULL, employee_ids LONGTEXT NOT NULL, status VARCHAR(32) NOT NULL, error_message VARCHAR(2000) NOT NULL DEFAULT '', created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_att_clock_state (clock_id VARCHAR(64) PRIMARY KEY, result_id VARCHAR(64) NOT NULL, batch_id VARCHAR(64) NOT NULL, work_date VARCHAR(10) NOT NULL, local_status VARCHAR(32) NOT NULL, basis VARCHAR(32) NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_overtime (id VARCHAR(64) PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, source VARCHAR(32) NOT NULL, external_ref VARCHAR(192) NULL, approval_status VARCHAR(32) NOT NULL, start_time DATETIME NOT NULL, end_time DATETIME NOT NULL, duration DECIMAL(12,2) NOT NULL, destination VARCHAR(255) NOT NULL DEFAULT '', reason VARCHAR(500) NOT NULL DEFAULT '', raw_json LONGTEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(source,external_ref))`,
	}
	statements = append(statements, checkinSnapshotDDL)
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	row, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 5).One()
	if err != nil {
		return err
	}
	if len(row) > 0 && row["checksum"].String() != checksum {
		return fmt.Errorf("schema version 5 checksum changed")
	}
	for _, s := range statements {
		if _, err = core.DB().Exec(ctx, s); err != nil {
			return fmt.Errorf("biz-suite schema version 5: %w", err)
		}
	}
	if len(row) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 5, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	return err
}

const checkinSnapshotDDL = `CREATE TABLE IF NOT EXISTS wecom_checkin_rule_snapshot (id VARCHAR(64) PRIMARY KEY, snapshot_type VARCHAR(32) NOT NULL, scope_key VARCHAR(192) NOT NULL, rule_date VARCHAR(10) NOT NULL DEFAULT '', start_time DATETIME NULL, end_time DATETIME NULL, raw_json LONGTEXT NOT NULL, fetched_at DATETIME NOT NULL)`
