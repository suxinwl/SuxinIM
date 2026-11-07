package main

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"log"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/internal/pluginworker"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/hrm"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/im"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/migration"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/organization"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/wecom"
)

func main() {
	pluginworker.Run("biz-suite", func(ctx context.Context, group *ghttp.RouterGroup) {
		if err := core.Init(g.DB()); err != nil {
			log.Panicf("biz-suite database: %v", err)
		}
		if err := migration.EnsureSchema(ctx, im.Schema()...); err != nil {
			log.Panicf("biz-suite database migration failed: %v", err)
		}
		if err := middleware.EnsureRuntimePluginRule(ctx, "biz-suite", "人事考勤与协同"); err != nil {
			log.Panicf("biz-suite menu: %v", err)
		}
		if err := core.Seed(ctx); err != nil {
			log.Panicf("biz-suite permissions: %v", err)
		}
		if err := organization.SeedRoles(ctx); err != nil {
			log.Panicf("biz-suite staff roles: %v", err)
		}
		client := wecom.NewFromEnv()
		if _, err := wecom.SyncFloor(); err != nil {
			log.Panicf("biz-suite activation time: %v", err)
		}
		wecom.SetAttendanceChangeHook(hrm.AttendanceChanged)
		if err := client.LoadSettings(ctx); err != nil {
			log.Panicf("biz-suite wecom settings: %v", err)
		}
		group.Group("/", func(api *ghttp.RouterGroup) {
			api.Middleware(core.Middleware)
			core.BindAccess(api)
			hrm.Bind(ctx, api)
			if !imbridge.ConfigFromEnv().Enabled {
				im.Bind(ctx, api)
			}
			imbridge.BindManager(api)
			wecom.Bind(ctx, api, client)
		})
		if err := wecom.RegisterScheduler(ctx, client); err != nil {
			log.Printf("biz-suite wecom scheduler: %v", err)
		}
		hrm.StartAttendanceWorker(ctx)
	})
}
