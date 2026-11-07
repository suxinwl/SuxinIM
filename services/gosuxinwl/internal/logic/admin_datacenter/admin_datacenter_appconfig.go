package admindatacenter

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取应用配置数据
func (s *sAdmindatacenter) GetConfig(ctx context.Context, req *datacenter.GetConfigReq) (res *gf.R) {
	appConfig, _ := g.Cfg("app").Get(ctx, "app")
	mapdata := appConfig.Map()
	res = gf.Success().SetMsg("获取应用配置").SetData(gf.Map{
		"vueobjroot":   mapdata["vueobjroot"],
		"loginCaptcha": mapdata["loginCaptcha"],
		"MultiLogin":   mapdata["MultiLogin"],
		"validityApi":  mapdata["validityApi"],
	})
	return
}

// 保存应用配置数据
func (s *sAdmindatacenter) SaveConfig(ctx context.Context, req *datacenter.SaveConfigReq) (res *gf.R) {
	err := gf.UpConfigFild("/manifest/config/app.yaml", gf.Map{
		"vueobjroot":   req.Vueobjroot,
		"MultiLogin":   req.MultiLogin,
		"loginCaptcha": req.LoginCaptcha,
		"validityApi":  req.ValidityApi,
	}, "  ")
	if err != nil {
		res = gf.Failed().SetMsg("保存应用配置数据失败！").SetData(err.Error())
		return
	}
	res = gf.Success().SetMsg("保存应用配置数据成功").SetData(true)
	return
}
