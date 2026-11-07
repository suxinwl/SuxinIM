package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Additive migration: leave the installed version 1 checksum and data intact.
func ensureV2(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS wecom_settings (id INTEGER NOT NULL PRIMARY KEY, settings_json TEXT NOT NULL, updated_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_hrm_attendance_trip (id VARCHAR(64) NOT NULL PRIMARY KEY, employee_id VARCHAR(64) NOT NULL, source VARCHAR(32) NOT NULL, external_ref VARCHAR(192) NULL, approval_status VARCHAR(32) NOT NULL, start_time DATETIME NOT NULL, end_time DATETIME NOT NULL, duration DECIMAL(12,2) NOT NULL, destination VARCHAR(255) NOT NULL DEFAULT '', reason VARCHAR(500) NOT NULL DEFAULT '', raw_json TEXT NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL, UNIQUE(source,external_ref))`,
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	previous, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 2).One()
	if err != nil {
		return err
	}
	if len(previous) > 0 && previous["checksum"].String() != checksum {
		return fmt.Errorf("schema version 2 checksum changed")
	}
	for _, sql := range statements {
		if _, err = core.DB().Exec(ctx, sql); err != nil {
			return fmt.Errorf("biz-suite schema version 2: %w", err)
		}
	}
	if len(previous) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 2, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	return err
}
