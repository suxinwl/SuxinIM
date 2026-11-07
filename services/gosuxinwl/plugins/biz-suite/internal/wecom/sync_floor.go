package wecom

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// A deployment may start fresh while retaining the WeCom connection. The floor
// applies equally to manual jobs, automatic jobs and the nightly 30-day scan.
func SyncFloor() (time.Time, error) {
	value := strings.TrimSpace(os.Getenv("GOSUXIN_SYNC_START_AT"))
	if value == "" {
		return time.Time{}, nil
	}
	stamp, err := core.ParseTime(value)
	if err != nil {
		return time.Time{}, errors.New("GOSUXIN_SYNC_START_AT 无效，请使用RFC3339或北京时间 YYYY-MM-DD HH:MM:SS")
	}
	return stamp, nil
}
func applySyncFloor(kind string, start, end time.Time) (time.Time, error) {
	if kind == "contact" {
		return start, nil
	}
	floor, err := SyncFloor()
	if err != nil {
		return start, err
	}
	if !floor.IsZero() && start.Before(floor) {
		start = floor
	}
	if !start.Before(end) {
		return start, errors.New("同步时间范围早于系统启用时间")
	}
	return start, nil
}
