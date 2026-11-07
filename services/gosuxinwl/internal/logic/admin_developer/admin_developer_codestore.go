// ================================
// 代码插件市场
// ================================
package admindeveloper

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 获取公共仓分类
func (c *sAdmindeveloper) GetCodeCate(ctx context.Context, req *developer.GetCodeCateReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().Get(ctx, req.Baseurl+"/goflycode/cate/getCate")
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区获取代码商店分类失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	path, _ := os.Getwd()
	downdir := filepath.Join(path, "/devsource/codemarket/release")
	privateHouse, _ := gcfg.Instance("app").Get(ctx, "app.PrivateHouse") // 私有仓地址
	gfversion, _ := gcfg.Instance("app").Get(ctx, "app.version")         // 框架版本号
	res = gf.Success().SetMsg("获取代码商城分类").SetData(gf.Map{
		"catedata":     parameter["data"],
		"privateHouse": privateHouse,
		"codepack":     downdir,
		"version":      gfversion,
	})
	return
}

// 获取公共仓数据
func (c *sAdmindeveloper) CodeList(ctx context.Context, req *developer.CodeListReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	reqdata := gconv.MapDeep(req, "p")
	delete(reqdata, "baseurl")
	result, err := g.Client().Get(ctx, req.Baseurl+"/goflycode/content/getCode", reqdata)
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区获取代码商店失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	data := parameter["data"].(map[string]interface{})
	list := data["items"].([]interface{})
	path, _ := os.Getwd()
	for _, val := range list {
		item := val.(map[string]interface{})
		installconfigPath := filepath.Join(path, "/manifest/codeinstall", gconv.String(item["name"]))
		if _, err := os.Stat(installconfigPath); os.IsNotExist(err) { //不存在
			item["is_install"] = false
		} else { //判断插件是否已经安装
			item["is_install"] = true
		}
	}
	res = gf.Success().SetMsg("获取代码商城分类").SetData(data)
	return
}

// 登录社区账号
func (c *sAdmindeveloper) Login(ctx context.Context, req *developer.LoginReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/user/login", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("登录社区账号成功").SetData(parameter["data"])
	return
}

// 更新私有仓地址
func (c *sAdmindeveloper) UpPrivateHouse(ctx context.Context, req *developer.UpPrivateHouseReq) (res *gf.R) {
	err := gf.UpConfigFild("/manifest/config/app.yaml", gf.Map{"PrivateHouse": req.PrivateHouse}, "  ")
	if err != nil {
		res = gf.Failed().SetMsg("更新私有仓地址失败！").SetData(err.Error())
		return
	}
	res = gf.Success().SetMsg("更新私有仓地址成功")
	return
}

// 检查框架版本更新
func (c *sAdmindeveloper) AsyncVersion(ctx context.Context, req *developer.AsyncVersionReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	gfversion, _ := gcfg.Instance("app").Get(ctx, "app.version") // 框架版本号
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/version/asyncVersion", gf.Map{
		"code_token": req.CodeToken, "version": gfversion, "from": "check", "frame": 2})
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("检查更新代码版本成功").SetData(parameter["data"])
	return
}

// 检查插件标识是否可用
func (c *sAdmindeveloper) CheckPackName(ctx context.Context, req *developer.CheckPackNameReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/ident/checkPackName", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("检测包名结果").SetData(parameter["data"])
	return
}

// 提交标识占用
func (c *sAdmindeveloper) SavePackName(ctx context.Context, req *developer.SavePackNameReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/ident/savePackName", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("提交标识占用成功").SetData(parameter["data"])
	return
}

// 发布插件需求到社区
func (c *sAdmindeveloper) Requirement(ctx context.Context, req *developer.RequirementReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	data := gconv.MapDeep(req, "p")
	delete(data, "baseurl")
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/content/save", data)
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("发布需求到社区成功").SetData(parameter["data"])
	return
}

// 获取文件路径
func (c *sAdmindeveloper) GetPackdirs(ctx context.Context, req *developer.GetPackdirsReq) (res *gf.R) {
	if req.Type == "go" {
		var option gf.DirOption
		option.RootPath = []string{"/api", "/internal", "/utility", "/resource"} // 目标根目录
		option.SubFlag = true                                                    // 遍历子目录标志 true: 遍历 false: 不遍历
		option.IgnorePath = []string{"service", "cmd", "consts", "packed", "router", "dao", "model", "gf", "auth",
			"baseapi", "baseapi", "dashboard", "install", "system", "user", "middleware",
			"admin_install", "admin_user", "basetool"} // 忽略目录
		option.IgnoreFile = []string{`.gitkeep`, `.gitignore`, "admin.go", "common.go",
			"admin_datacenter_attachment.go", "del_tree.go", "loger.go", "system_account.go", "system_role.go", "adminBase.go",
			"admin_common_message.go", "admin_dashboard_workplace.go", "admin_datacenter_appconfig.go", "admin_datacenter_configuration.go", "admin_datacenter_dictionary.go", "admin_datacenter_tabledata.go", "admin_datacenter_upfile.go",
			"admin_datacenter_uploadconfig.go", "admin_developer_codestore.go", "admin_developer_packinstall.go", "admin_install_index.go", "admin_new.go", "admin_router.go", "admin_system_account.go", "admin_system_dept.go", "admin_system_log.go",
			"admin_system_role.go", "admin_system_rule.go", "admin_user.go", "logic.go", "codestore.go", "packinstall.go", "admin_developer.go",
			"admin_developer_config.go",
		} // 忽略文件
		appDir, err := gf.TraverDir(option)
		if err != nil {
			res = gf.Failed().SetMsg("获取后端目录失败").SetData(err)
			return
		}
		res = gf.Success().SetMsg("获取后端文件路径").SetData(appDir)
	} else {
		//前端
		vueobjroot, _ := g.Cfg("app").Get(ctx, "app.vueobjroot")
		var option gf.DirOption
		option.RootPath = []string{"/src", "/public"} // 目标根目录
		option.SubFlag = true                         // 遍历子目录标志 true: 遍历 false: 不遍历
		option.IgnorePath = []string{"api", "router", "directive", "systool", "configuration", "dictionary",
			"account", "log", "dept", "role", "rule", "usersetting", "codestore"} // 忽略目录
		option.IgnoreFile = []string{"App.vue", "main.ts", "env.d.ts", "common.go", "settings.json"} // 忽略文件
		appDir, err := gf.TraverVueDir(option, vueobjroot.String())
		if err != nil {
			res = gf.Failed().SetMsg("获取前端目录失败").SetData(err)
			return
		}
		appDir.Path = vueobjroot.String()
		res = gf.Success().SetMsg("获取前端文件路径").SetData(appDir)
	}
	return
}

// 获取后台菜单
func (c *sAdmindeveloper) GetMenutree(ctx context.Context, req *developer.GetMenutreeReq) (res *gf.R) {
	business_menuList, _ := dao.AuthRule.Ctx(ctx).Fields("id,pid,title,locale").Order("weigh asc").All()
	if business_menuList == nil {
		business_menuList = make(gf.OrmResult, 0)
	}
	for _, val := range business_menuList {
		if val["title"].String() == "" {
			val["title"] = val["locale"]
		}
	}
	business_menuList = gf.GetTreeArray(business_menuList, 0, "")
	res = gf.Success().SetMsg("获取后台菜单").SetData(business_menuList)
	return
}

// 菜单id转JSON数据
func (c *sAdmindeveloper) MenuTreeToJson(ctx context.Context, req *developer.MenuTreeToJsonReq) (res *gf.R) {
	rules := clogic.GetRulesID("auth_rule", "pid", req.Menu) //获取子菜单包含的父级ID
	menuList, _ := dao.AuthRule.Ctx(ctx).WhereIn("id", rules.([]interface{})).
		Fields("id,pid,title,locale,type,icon,routepath,routename,component,permission,path,redirect,isExt,keepalive,hideInMenu,activeMenu,noAffix,onlypage,requiresAuth").Order("weigh asc").All()
	if menuList == nil {
		menuList = make(gf.OrmResult, 0)
	}
	for _, val := range menuList {
		if val["title"].String() == "" {
			val["title"] = val["locale"]
		}
	}
	menuList = clogic.GetRuleTreeArrayByPack(menuList, 0)
	res = gf.Success().SetMsg("菜单id转JSON数据").SetData(menuList)
	return
}

// 发布插件到代码仓
func (c *sAdmindeveloper) UpPackToService(ctx context.Context, req *developer.UpPackToServiceReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	if req.Baseurl == "" {
		res = gf.Failed().SetMsg("代码仓地址不存在！")
		return
	}
	data := gconv.MapDeep(req, "p")
	delete(data, "baseurl")
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/content/save", data)
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("发布插件到代码仓成功").SetData(parameter["data"])
	return
}

// 获取邮箱验证码
func (c *sAdmindeveloper) LoginCode(ctx context.Context, req *developer.LoginCodeReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/user/loginCode", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("获取验证码成功").SetData(parameter["data"])
	return
}

// 免密登录
func (c *sAdmindeveloper) FreeLogin(ctx context.Context, req *developer.FreeLoginReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/user/freeLogin", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("登录成功").SetData(parameter["data"])
	return
}

// 注册账号
func (c *sAdmindeveloper) RegisterUser(ctx context.Context, req *developer.RegisterUserReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	result, err := g.Client().ContentJson().Post(ctx, req.Baseurl+"/goflycode/user/registerUser", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	res = gf.Success().SetMsg("注册成功").SetData(parameter["data"])
	return
}
