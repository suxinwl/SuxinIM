//go:build !android && !ios && !js

package db

import (
	"database/sql"
	"github.com/openimsdk/tools/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"time"
)

func (d *DataBase) openLocalWorkingDB(path string, level logger.LogLevel) (*gorm.DB, error) {
	return gorm.Open(sqlite.Open(path), &gorm.Config{Logger: log.NewSqlLogger(level, false, time.Millisecond*200)})
}
func (d *DataBase) installLocalArchive(_, _ string) error { return nil }
func (d *DataBase) startLocalArchive(_ []string) error    { return nil }
func (d *DataBase) stopLocalArchive()                     {}

func (d *DataBase) closeSDKWorkingDatabase(connection *sql.DB) error {
	return connection.Close()
}
