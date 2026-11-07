package core

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"
)

var Location = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic(err)
	}
	return loc
}()
var mu sync.RWMutex
var business, host gdb.DB

// Init opens the host's database with a separate, unprefixed business model.
// Host tables retain their configured prefix and timestamp conventions.
func Init(h gdb.DB) error {
	cfg := *h.GetConfig()
	cfg.Prefix = ""
	cfg.CreatedAt = ""
	cfg.UpdatedAt = ""
	cfg.DeletedAt = ""
	cfg.Debug = false
	db, err := gdb.New(cfg)
	if err != nil {
		return err
	}
	mu.Lock()
	business = db
	host = h
	mu.Unlock()
	return nil
}
func DB() gdb.DB {
	mu.RLock()
	defer mu.RUnlock()
	if business == nil {
		panic("biz-suite database is not initialized")
	}
	return business
}
func Model(table string) *gdb.Model     { return DB().Model(table) }
func HostModel(table string) *gdb.Model { mu.RLock(); h := host; mu.RUnlock(); return h.Model(table) }
func UID(ctx context.Context) int64     { return gconv.Int64(ctx.Value("uid")) }
func HostUserExists(ctx context.Context, id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	return HostModel("admin").Ctx(ctx).Where("id", id).Where("status", 0).Exist()
}
func Now() string { return time.Now().In(Location).Format("2006-01-02 15:04:05") }
func ParseTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, e := time.ParseInLocation(layout, value, Location); e == nil {
			return t.In(Location), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date/time %q", value)
}
