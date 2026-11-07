// Package dbcompat keeps database metadata queries independent of SQL dialect.
package dbcompat

import (
	"context"
	"sort"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
)

func Columns(ctx context.Context, db gdb.DB, table string) ([]*gdb.TableField, error) {
	fields, err := db.TableFields(ctx, table)
	if err != nil {
		return nil, err
	}
	result := make([]*gdb.TableField, 0, len(fields))
	for _, field := range fields {
		result = append(result, field)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Index < result[j].Index })
	return result, nil
}

func Tables(ctx context.Context, db gdb.DB) ([]map[string]any, error) {
	result := make([]map[string]any, 0)
	if db.GetConfig().Type == "mysql" {
		rows, err := db.Query(ctx, "SELECT TABLE_NAME,TABLE_COMMENT,ENGINE,TABLE_COLLATION FROM information_schema.tables WHERE table_schema=? ORDER BY TABLE_NAME", db.GetConfig().Name)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			result = append(result, map[string]any{"name": row["TABLE_NAME"].String(), "title": row["TABLE_COMMENT"].String(), "engine": row["ENGINE"].String(), "collation": row["TABLE_COLLATION"].String()})
		}
		return result, nil
	}
	tables, err := db.Tables(ctx)
	if err != nil {
		return nil, err
	}
	for _, table := range tables {
		if strings.HasPrefix(table, "sqlite_") {
			continue
		}
		result = append(result, map[string]any{"name": table, "title": table, "engine": "SQLite", "collation": ""})
	}
	return result, nil
}
