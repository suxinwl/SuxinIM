package clogic

import (
	"context"

	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/model/do"
	"github.com/suxinwl/GoSuxin/utility/plugin"

	"github.com/mssola/user_agent"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 添加登录日志
func AddloginLog(ctx context.Context, savedata g.Map) {
	ip := g.RequestFromCtx(ctx).GetClientIp()
	if ip == "::1" {
		ip = "127.0.0.1"
	}
	address, _ := plugin.NewIpRegion(ip)
	ua := user_agent.New(ghttp.RequestFromCtx(ctx).Header.Get("User-Agent"))
	browser, _ := ua.Browser()
	loginData := &do.LoginLog{
		Uid:      savedata["uid"],
		Username: savedata["username"],
		Ip:       ip,
		Address:  address,
		Browser:  browser,
		Os:       ua.OS(),
		Status:   savedata["status"],
		Des:      savedata["des"],
		ErrorMsg: gconv.String(savedata["error_msg"]),
	}
	if _, err := dao.LoginLog.Ctx(ctx).Insert(loginData); err != nil {
		g.Log().Warning(ctx, "登录记录写入失败", err)
	}
}
