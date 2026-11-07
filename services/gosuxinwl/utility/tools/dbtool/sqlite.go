package dbtool

import (
	"database/sql"
	"fmt"
	"io"
	"strings"
	"time"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
)

func sqliteIdentifier(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func dumpSQLite(db *sql.DB, out io.Writer, tables []string, prefix string, data, drop bool) error {
	// One read transaction keeps schema and rows in the same snapshot.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(tables) == 0 {
		rows, err := tx.Query("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name")
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			if err = rows.Scan(&name); err != nil {
				rows.Close()
				return err
			}
			tables = append(tables, name)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if _, err = fmt.Fprintln(out, "-- SQLite database dump; import with SQLite.\nBEGIN TRANSACTION;"); err != nil {
		return err
	}
	for _, table := range tables {
		var ddl string
		if err = tx.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&ddl); err != nil {
			return err
		}
		target := strings.TrimPrefix(table, prefix)
		// Keep the column definitions verbatim; normalize only the table identifier.
		start := strings.Index(ddl, "(")
		if start < 0 {
			return fmt.Errorf("unsupported SQLite table: %s", table)
		}
		if drop {
			if _, err = fmt.Fprintf(out, "DROP TABLE IF EXISTS %s;\n", sqliteIdentifier(target)); err != nil {
				return err
			}
		}
		if _, err = fmt.Fprintf(out, "CREATE TABLE IF NOT EXISTS %s %s;\n", sqliteIdentifier(target), ddl[start:]); err != nil {
			return err
		}
		if data {
			rows, err := tx.Query("SELECT * FROM " + sqliteIdentifier(table))
			if err != nil {
				return err
			}
			columns, err := rows.Columns()
			if err != nil {
				rows.Close()
				return err
			}
			for rows.Next() {
				values := make([]any, len(columns))
				pointers := make([]any, len(columns))
				for i := range values {
					pointers[i] = &values[i]
				}
				if err = rows.Scan(pointers...); err != nil {
					rows.Close()
					return err
				}
				literals := make([]string, len(values))
				for i, value := range values {
					literals[i], err = sqliteLiteral(value)
					if err != nil {
						rows.Close()
						return err
					}
				}
				if _, err = fmt.Fprintf(out, "INSERT INTO %s VALUES (%s);\n", sqliteIdentifier(target), strings.Join(literals, ",")); err != nil {
					rows.Close()
					return err
				}
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return err
			}
		}
		// Preserve explicit indexes and triggers as well as inline constraints.
		rows, err := tx.Query("SELECT sql FROM sqlite_master WHERE tbl_name=? AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name", table)
		if err != nil {
			return err
		}
		for rows.Next() {
			var statement string
			if err = rows.Scan(&statement); err != nil {
				rows.Close()
				return err
			}
			if prefix != "" {
				for _, quote := range []string{"`", `"`, "["} {
					statement = strings.ReplaceAll(statement, quote+prefix, quote)
				}
			}
			if _, err = fmt.Fprintln(out, statement+";"); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintln(out, "COMMIT;")
	return err
}

func sqliteLiteral(value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "NULL", nil
	case int64:
		return fmt.Sprint(v), nil
	case float64:
		return fmt.Sprint(v), nil
	case bool:
		if v {
			return "1", nil
		}
		return "0", nil
	case []byte:
		return fmt.Sprintf("X'%X'", v), nil
	case time.Time:
		return "'" + v.Format("2006-01-02 15:04:05.999999999-07:00") + "'", nil
	case string:
		if strings.ContainsRune(v, 0) {
			return fmt.Sprintf("CAST(X'%X' AS TEXT)", []byte(v)), nil
		}
		return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
	default:
		return "", fmt.Errorf("unsupported SQLite value %T", value)
	}
}
