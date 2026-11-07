package tools

import (
	"context"
	"testing"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/robfig/cron/v3"
)

func TestPermanentRetentionRegistersNoCleanupJobs(t *testing.T) {
	cfg := config.CronTask{CronExecuteTime: "0 2 * * *", RetainChatRecords: 0, FileExpireTime: 0, DeleteObjectType: []string{"msg-file"}}
	if err := validateRetention(cfg); err != nil {
		t.Fatal(err)
	}
	s := &cronServer{ctx: context.Background(), config: &CronTaskConfig{CronTask: cfg}, cron: cron.New()}
	for _, register := range []func() error{s.registerClearS3, s.registerDeleteMsg, s.registerClearUserMsg} {
		if err := register(); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.cron.Entries()) != 0 {
		t.Fatal("permanent configuration scheduled destructive cleanup")
	}
}

func TestRetentionRejectsNegativeButKeepsExplicitPositiveSchedule(t *testing.T) {
	for _, cfg := range []config.CronTask{{RetainChatRecords: -1}, {FileExpireTime: -1}} {
		if validateRetention(cfg) == nil {
			t.Fatal("negative retention accepted")
		}
	}
	cfg := config.CronTask{CronExecuteTime: "0 2 * * *", RetainChatRecords: 365, FileExpireTime: 180, DeleteObjectType: []string{"msg-file"}}
	s := &cronServer{ctx: context.Background(), config: &CronTaskConfig{CronTask: cfg}, cron: cron.New()}
	if err := validateRetention(cfg); err != nil {
		t.Fatal(err)
	}
	for _, register := range []func() error{s.registerClearS3, s.registerDeleteMsg, s.registerClearUserMsg} {
		if err := register(); err != nil {
			t.Fatal(err)
		}
	}
	if len(s.cron.Entries()) != 3 {
		t.Fatal("explicit positive retention no longer schedules its cleanup jobs")
	}
}
