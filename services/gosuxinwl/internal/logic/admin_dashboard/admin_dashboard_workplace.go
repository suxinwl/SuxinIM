// ================================================================================
// 注意如果需要通过logic生成service名称type名称前面要s开头
// 所有小写 s 开头，大写字母随后的结构体都将被当做业务模块接口名称,例如：sAdminUser
// 生成servic命令：suxin gen service
// 具体参考：https://www.suxinwl.com/docs/gosuxin/v1.0.0/cli/gen-service
// ================================================================================
package admindashboard

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/dashboard"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/internal/service"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	sAdminDashboard struct{}
)

func init() {
	service.RegisterAdminDashboard(NewAdminDashboard())
}

func NewAdminDashboard() service.IAdminDashboard {
	return &sAdminDashboard{}
}

// 获取快捷操作数据
func (s *sAdminDashboard) GetQuick(ctx context.Context, req *dashboard.GetQuickReq) (res *gf.R) {
	list, err := dao.HomeQuickop.Ctx(ctx).All()
	if err != nil {
		res = gf.Failed().SetMsg("获取快捷操作数据失败").SetData(err)
		return
	}
	visible := list[:0]
	for _, item := range list {
		if plugins.EntryEnabled(item["path_url"].String()) {
			visible = append(visible, item)
		}
	}
	res = gf.Success().SetMsg("获取管理后台菜单").SetData(visible)
	return
}

// 保存快捷操作数据
func (s *sAdminDashboard) SaveQuick(ctx context.Context, req *dashboard.SaveQuickReq) (res *gf.R) {
	if req.Id == gf.ZeroValue {
		addId, err := dao.HomeQuickop.Ctx(ctx).Data(req).FieldsEx("id").InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加失败").SetData(err)
		} else {
			if addId != 0 {
				dao.HomeQuickop.Ctx(ctx).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			}
			res = gf.Success().SetMsg("添加成功！")
		}
	} else {
		result, err := dao.HomeQuickop.Ctx(ctx).Data(req).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	return
}
