package migration

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// Ownership records keep synchronization from taking over pre-existing host
// accounts/departments. Removing the plugin never removes host identities.
func ensureV4(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS biz_org_binding (entity_type VARCHAR(32) NOT NULL, external_id VARCHAR(128) NOT NULL, host_id BIGINT NOT NULL, created_at DATETIME NOT NULL, PRIMARY KEY(entity_type,external_id), UNIQUE(entity_type,host_id))`,
		`CREATE TABLE IF NOT EXISTS biz_org_role (role_key VARCHAR(32) NOT NULL PRIMARY KEY, role_id BIGINT NOT NULL UNIQUE, created_at DATETIME NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS biz_org_sync_state (id INTEGER NOT NULL PRIMARY KEY, report_json TEXT NOT NULL, updated_at DATETIME NOT NULL)`,
	}
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(statements, "\n"))))
	row, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 4).One()
	if err != nil {
		return err
	}
	if len(row) > 0 && row["checksum"].String() != checksum {
		return fmt.Errorf("schema version 4 checksum changed")
	}
	for _, statement := range statements {
		if _, err = core.DB().Exec(ctx, statement); err != nil {
			return fmt.Errorf("biz-suite schema version 4: %w", err)
		}
	}
	if len(row) == 0 {
		if _, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 4, "checksum": checksum, "applied_at": core.Now()}).Insert(); err != nil {
			return err
		}
	}
	exists, err := core.Model("biz_org_sync_state").Ctx(ctx).Where("id", 1).Exist()
	if err != nil {
		return err
	}
	if !exists {
		_, err = core.Model("biz_org_sync_state").Ctx(ctx).Data(g.Map{"id": 1, "report_json": "{}", "updated_at": core.Now()}).Insert()
	}
	return err
}
