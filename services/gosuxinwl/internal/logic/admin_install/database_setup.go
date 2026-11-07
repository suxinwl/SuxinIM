package admininstall

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	"github.com/suxinwl/GoSuxin/framework/util/grand"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// The existing generated DAOs and plugin SDK use gf_ tables.
func normalizeDatabase(values map[string]string) (map[string]string, error) {
	v := make(map[string]string, len(values))
	for key, value := range values {
		v[key] = value
	}
	switch v["type"] {
	case "", "mysql", "mysql8":
		v["type"] = "mysql"
		if v["version"] == "" {
			v["version"] = "8.0"
		}
	case "mysql57":
		v["type"], v["version"] = "mysql", "5.7"
	case "sqlite":
		v["version"] = ""
	default:
		return nil, fmt.Errorf("请选择 MySQL 8、MySQL 5.7 或 SQLite")
	}
	if v["prefix"] == "" {
		v["prefix"] = "gf_"
	}
	if v["prefix"] != "gf_" {
		return nil, fmt.Errorf("当前系统及插件使用固定表前缀 gf_")
	}
	if v["type"] == "sqlite" {
		if v["path"] == "" {
			v["path"] = "storage/database/gosuxin.db"
		}
		if strings.ContainsAny(v["path"], "?\x00") || v["path"] == ":memory:" || strings.HasPrefix(v["path"], "file:") {
			return nil, fmt.Errorf("SQLite 请填写持久化数据库文件路径")
		}
		path, err := filepath.Abs(v["path"])
		if err != nil {
			return nil, err
		}
		v["name"] = filepath.ToSlash(path)
		v["extra"] = "busy_timeout=5000&journal_mode=WAL&foreign_keys=ON"
		for _, key := range []string{"host", "port", "user", "pass"} {
			v[key] = ""
		}
	} else {
		if v["version"] != "8.0" && v["version"] != "5.7" {
			return nil, fmt.Errorf("不支持的 MySQL 版本")
		}
		if v["host"] == "" || v["user"] == "" {
			return nil, fmt.Errorf("请填写 MySQL 地址和用户名")
		}
		if v["port"] == "" {
			v["port"] = "3306"
		}
		if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(v["name"]) {
			return nil, fmt.Errorf("数据库名称只能包含字母、数字和下划线")
		}
		v["extra"] = ""
	}
	return v, nil
}

func mysqlDSN(v map[string]string, database string) string {
	c := mysql.NewConfig()
	c.User, c.Passwd, c.Net = v["user"], v["pass"], "tcp"
	c.Addr, c.DBName = net.JoinHostPort(v["host"], v["port"]), database
	c.ParseTime, c.MultiStatements, c.Loc = true, true, time.Local
	c.Timeout = 5 * time.Second
	c.Params = map[string]string{"charset": "utf8mb4", "sql_mode": "'NO_ENGINE_SUBSTITUTION'"}
	return c.FormatDSN()
}

// Only an empty database can be initialized. Never run the seed's DROP TABLEs
// against an existing installation, even when its local lock is missing.
func openInstallDatabase(ctx context.Context, v map[string]string) (*sql.DB, error) {
	var db *sql.DB
	var err error
	if v["type"] == "sqlite" {
		if err = os.MkdirAll(filepath.Dir(v["name"]), 0700); err != nil {
			return nil, err
		}
		db, err = sql.Open("sqlite", v["name"]+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	} else {
		server, e := sql.Open("mysql", mysqlDSN(v, ""))
		if e != nil {
			return nil, e
		}
		defer server.Close()
		var version string
		if e = server.QueryRowContext(ctx, "SELECT VERSION()").Scan(&version); e != nil {
			return nil, e
		}
		wanted := "8."
		if v["version"] == "5.7" {
			wanted = "5.7."
		}
		if !strings.HasPrefix(version, wanted) || strings.Contains(strings.ToLower(version), "mariadb") {
			return nil, fmt.Errorf("所选 MySQL %s 与服务器版本 %s 不匹配", v["version"], version)
		}
		if _, e = server.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+v["name"]+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci"); e != nil {
			return nil, e
		}
		db, err = sql.Open("mysql", mysqlDSN(v, v["name"]))
	}
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err = db.PingContext(ctx); err == nil {
		var count int
		if v["type"] == "sqlite" {
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&count)
		} else {
			err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=?", v["name"]).Scan(&count)
		}
		if err == nil && count != 0 {
			err = fmt.Errorf("目标数据库已有数据表，请选择空数据库；安装器不会覆盖已有数据")
		}
	}
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func seedDatabase(ctx context.Context, db *sql.DB, kind, directory, username, password string) error {
	name := "gosuxin.sql"
	if kind == "sqlite" {
		name = "sqlite.sql"
	}
	content, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, string(content)); err != nil {
		return fmt.Errorf("初始化数据库: %w", err)
	}
	salt := grand.Str("123456789", 6)
	hash := gf.Md5(fmt.Sprintf("%v%v", gf.Md5(password), salt))
	if _, err = tx.ExecContext(ctx, "UPDATE gf_admin SET username=?, password=?, salt=? WHERE id=1", username, hash, salt); err != nil {
		return err
	}
	return tx.Commit()
}
