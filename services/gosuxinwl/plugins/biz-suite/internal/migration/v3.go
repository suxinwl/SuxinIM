package migration

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func ensureV3(ctx context.Context) error {
	const statement = `CREATE TABLE IF NOT EXISTS wecom_credentials (scope VARCHAR(32) NOT NULL PRIMARY KEY, encrypted_secret TEXT NOT NULL, updated_at DATETIME NOT NULL)`
	checksum := fmt.Sprintf("%x", sha256.Sum256([]byte(statement)))
	row, err := core.Model("biz_schema_version").Ctx(ctx).Where("version", 3).One()
	if err != nil {
		return err
	}
	if len(row) > 0 && row["checksum"].String() != checksum {
		return fmt.Errorf("schema version 3 checksum changed")
	}
	if _, err = core.DB().Exec(ctx, statement); err != nil {
		return fmt.Errorf("biz-suite schema version 3: %w", err)
	}
	if len(row) == 0 {
		_, err = core.Model("biz_schema_version").Ctx(ctx).Data(g.Map{"version": 3, "checksum": checksum, "applied_at": core.Now()}).Insert()
	}
	return err
}
