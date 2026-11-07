package wecom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func now() string           { return core.Now() }
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func stableID(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func dbError(operation string) error {
	return fmt.Errorf("wecom %s database operation failed", operation)
}

// upsert preserves the primary key; the caller holds the sync lock and table
// uniqueness constraints protect accidental duplicate writes across processes.
func upsert(ctx context.Context, table, key string, value any, data g.Map) error {
	row, err := core.Model(table).Ctx(ctx).Where(key, value).One()
	if err != nil {
		return dbError(table)
	}
	if len(row) > 0 {
		delete(data, "id")
		delete(data, "created_at")
		_, err = core.Model(table).Ctx(ctx).Where(key, value).Update(data)
	} else {
		_, err = core.Model(table).Ctx(ctx).Data(data).Insert()
	}
	if err != nil {
		return dbError(table)
	}
	return nil
}
func employeeIDByWeCom(ctx context.Context, uid string) (string, error) {
	if uid == "" {
		return "", errors.New("approval or checkin has no applicant userid")
	}
	rows, err := core.Model("biz_hrm_employee").Ctx(ctx).Where("wecom_userid", uid).Limit(2).All()
	if err != nil {
		return "", dbError("employee lookup")
	}
	if len(rows) != 1 {
		return "", errors.New("enterprise WeChat userid has no unique employee mapping; synchronize or correct contacts first")
	}
	return rows[0]["id"].String(), nil
}
func windows(start, end time.Time, maximum time.Duration) ([][2]time.Time, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return nil, errors.New("end_time must be greater than start_time")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("one sync request cannot exceed 366 days; import older history in separate batches")
	}
	var result [][2]time.Time
	for start.Before(end) {
		next := start.Add(maximum)
		if next.After(end) {
			next = end
		}
		result = append(result, [2]time.Time{start, next})
		start = next
	}
	return result, nil
}

type PartialError struct{ Failed int }

func (e *PartialError) Error() string {
	return fmt.Sprintf("%d records could not be normalized; inspect sync exceptions", e.Failed)
}
func partial(failed int) error {
	if failed > 0 {
		return &PartialError{failed}
	}
	return nil
}
