package adminuser

import (
	"time"

	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// A newly installed account has SQL NULL in lock_time and is not locked.
func accountIsLocked(lockTime *gtime.Time, now time.Time) bool {
	return lockTime != nil && now.Before(lockTime.Time)
}
