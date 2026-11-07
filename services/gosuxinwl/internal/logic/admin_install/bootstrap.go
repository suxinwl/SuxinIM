package admininstall

import (
	"context"
	"fmt"
	"strings"
)

// BootstrapEmpty installs only system metadata and one explicitly credentialed
// administrator. It never imports an old database or unpacks frontend sources.
func BootstrapEmpty(ctx context.Context, config map[string]string, seedDirectory, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 20 || len(password) < 16 {
		return fmt.Errorf("管理员账号不可为空或超过20字符，密码至少16字符")
	}
	values, err := normalizeDatabase(config)
	if err != nil {
		return err
	}
	db, err := openInstallDatabase(ctx, values)
	if err != nil {
		return err
	}
	defer db.Close()
	if err = seedDatabase(ctx, db, values["type"], seedDirectory, username, password); err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"gf_auth_dept", "gf_dictionary_data", "gf_dictionary_group", "gf_home_quickop"} {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "UPDATE gf_admin SET dept_id=0,name=?,nickname=?,avatar=?,email='',mobile='',tel='' WHERE id=1", "系统管理员", "系统管理员", "/resource/static/brand/default-avatar.png?v=puty2"); err != nil {
		return err
	}
	return tx.Commit()
}
