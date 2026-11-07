package adminuser

import (
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

func TestAccountLockTime(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		until  *gtime.Time
		locked bool
	}{
		{"fresh install SQL NULL", nil, false},
		{"zero date", gtime.New(time.Time{}), false},
		{"expired lock", gtime.New(now.Add(-time.Minute)), false},
		{"expires now", gtime.New(now), false},
		{"active lock", gtime.New(now.Add(time.Minute)), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountIsLocked(tc.until, now); got != tc.locked {
				t.Fatalf("accountIsLocked = %v, want %v", got, tc.locked)
			}
		})
	}
}
