// ================================================================================
// 注意如果需要通过logic生成service名称type名称前面要s开头
// 所有小写 s 开头，大写字母随后的结构体都将被当做业务模块接口名称,例如：sAdminUser
// 生成servic命令：suxin gen service
// 具体参考：https://www.suxinwl.com/docs/gosuxin/v1.0.0/cli/gen-service
// ================================================================================
package adminuser

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/user"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
	"github.com/suxinwl/GoSuxin/internal/service"
	"github.com/suxinwl/GoSuxin/utility/auth"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"github.com/suxinwl/GoSuxin/utility/tools/cryptojs"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

type (
	sAdminUser struct{}
)

func init() {
	service.RegisterAdminUser(NewAdminUser())
}

func NewAdminUser() service.IAdminUser {
	return &sAdminUser{}
}

// 实现登录业务处理
func (s *sAdminUser) Login(ctx context.Context, req *user.LoginReq) (res *gf.R) {
	data := entity.Admin{}
	if !g.IsEmpty(req.Username) {
		authenticated, authErr := s.AuthenticatePassword(ctx, req.Username, req.Password, req.Codeid, req.Captcha)
		if authErr != nil {
			return gf.Failed().SetMsg(authErr.Error())
		}
		data = *authenticated
		security, err := accountsecurity.Default(ctx)
		if err != nil {
			return gf.Failed().SetMsg("账号安全服务暂不可用")
		}
		state, err := security.State(ctx, int64(data.Id))
		if err != nil {
			return gf.Failed().SetMsg("账号安全服务暂不可用")
		}
		if state.MustChange {
			challenge, err := security.Challenge(ctx, int64(data.Id), data.AuthenticatedEpoch)
			if err != nil {
				return gf.Failed().SetMsg("账号验证未通过")
			}
			return gf.Failed().SetCode(428).SetMsg("请先修改密码").SetData(challenge)
		}
		token, err := generateHRMLoginToken(ctx, data)
		if err != nil {
			return gf.Failed().SetMsg(hrmLoginTokenError(err))
		}
		dao.Admin.Ctx(ctx).WherePri(data.Id).Update(g.Map{dao.Admin.Columns().Loginip: g.RequestFromCtx(ctx).GetClientIp(), dao.Admin.Columns().Logintime: gtime.Now()})
		clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": req.Username, "status": 0, "des": "账号登录"})
		token, err = cryptojs.AesEncrypt(token)
		if err != nil {
			return gf.Failed().SetMsg("登录失败")
		}
		res = gf.Success().SetData(token).SetMsg("登录成功")
	} else if !g.IsEmpty(req.Email) { //手机登录
		err := dao.Admin.Ctx(ctx).Where("email", req.Email).Scan(&data)
		if nil == err {
			if data.Status == gf.StatusNo {
				res = gf.Failed().SetMsg("账号被禁用了")
				return
			}
			if accountIsLocked(data.LockTime, time.Now()) {
				res = gf.Failed().SetMsg("账户已被锁定，请稍后再试")
				return
			}
			code, emerr := gf.GetVerifyCode(req.Email)
			//检查是否验证码错误超过3次
			if emerr != nil || code != req.Captcha {
				clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": req.Username, "status": 1, "des": "邮箱登录", "error_msg": "验证码无效"})

				res = gf.Failed().SetMsg("验证码无效")
				return
			}
			//创建token
			token, err := generateHRMLoginToken(ctx, data)
			if err != nil {
				res = gf.Failed().SetMsg(hrmLoginTokenError(err))
			} else {
				dao.Admin.Ctx(ctx).WherePri(data.Id).Unscoped().Update(g.Map{
					dao.Admin.Columns().Loginip:   g.RequestFromCtx(ctx).GetClientIp(),
					dao.Admin.Columns().Logintime: gtime.Now(),
				})
				clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": req.Username, "status": 0, "des": "邮箱登录"})
				token, _ = cryptojs.AesEncrypt(token)
				res = gf.Success().SetData(token).SetMsg("登录成功")
			}
		} else {
			res = gf.Failed().SetMsg("账号不存在！")
		}
	} else if !g.IsEmpty(req.Mobile) { //电话号码登录
		err := dao.Admin.Ctx(ctx).Where("mobile", req.Mobile).Scan(&data)
		if nil == err {
			if data.Status == gf.StatusNo {
				res = gf.Failed().SetMsg("账号被禁用了")
				return
			}
			if accountIsLocked(data.LockTime, time.Now()) {
				res = gf.Failed().SetMsg("账户已被锁定，请稍后再试")
				return
			}
			code, emerr := gf.GetVerifyCode(req.Mobile)
			// 验证码错误不计入密码错误窗口。
			if emerr != nil || code != req.Captcha {
				clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": req.Username, "status": 1, "des": "手机号登录", "error_msg": "验证码无效"})

				res = gf.Failed().SetMsg("验证码无效")
				return
			}
			//创建token
			token, err := generateHRMLoginToken(ctx, data)
			if err != nil {
				res = gf.Failed().SetMsg(hrmLoginTokenError(err))
			} else {
				dao.Admin.Ctx(ctx).WherePri(data.Id).Unscoped().Update(g.Map{
					dao.Admin.Columns().Loginip:   g.RequestFromCtx(ctx).GetClientIp(),
					dao.Admin.Columns().Logintime: gtime.Now(),
				})
				clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": req.Username, "status": 0, "des": "手机号登录"})
				token, _ = cryptojs.AesEncrypt(token)
				res = gf.Success().SetData(token).SetMsg("登录成功")
			}
		} else {
			res = gf.Failed().SetMsg("账号不存在！")
		}
	} else {
		res = gf.Failed().SetMsg("请填写账号、使用邮箱或电话登录")
	}
	return
}

// 获取用户信息
func (s *sAdminUser) GetUserinfo(ctx context.Context, req *user.GetUserinfoReq) (res *gf.R) {
	uid := ctx.Value("uid")
	MDB := dao.Admin.Ctx(ctx).Where("id", uid)
	if ok, err := MDB.Exist(); !ok || err != nil {
		res = gf.Failed().SetMsg("账号不存在！")
		return
	}
	data, err := MDB.Fields("id,dept_id,username,name,nickname,email,mobile,avatar,status,createtime,pwd_reset_time").One()
	if err != nil {
		res = gf.Failed().SetMsg("账号信息异常！").SetData(err)
		return
	}
	if data["avatar"].IsEmpty() {
		data["avatar"].Set("/resource/static/brand/default-avatar.png?v=puty2")
	}
	if data["name"].IsEmpty() {
		data["name"].Set(data["nickname"])
	}
	if !data["dept_id"].IsEmpty() && data["dept_id"].Int64() > gf.ZeroValue {
		deptname, _ := dao.AuthDept.Ctx(ctx).Cache(gdb.CacheOption{
			Duration: time.Hour, //将查询结果缓存1小时
			Name:     "sys-dept",
			Force:    false,
		}).Where("id", data["dept_id"]).Value("name")
		data["deptname"] = deptname
	}
	roles, _ := dao.AuthRoleAccess.Ctx(ctx).Cache(gdb.CacheOption{
		Duration: time.Hour, //将查询结果缓存1小时
		Name:     "sys-dept",
		Force:    false,
	}).As("a").LeftJoin("business_auth_role", "r", "r.id=a.role_id").Where("a.uid", uid).Array("r.name")
	if len(roles) > 0 {
		data["roles"] = g.NewVar(strings.Join(gf.Strings(roles), "，"))
	}
	//处理敏感信息
	data["mobile"].Set(gf.HideStrInfo("mobile", data["mobile"].String()))
	data["email"].Set(gf.HideStrInfo("email", data["email"].String()))
	//附件访问完整地址域名
	data["rooturls"] = g.NewVar(gf.GetAllRootUrl()) //全部上传方式访问地址
	data["defrooturl"] = g.NewVar(gf.GetRootUrl())  //设置的上传方式访问地址
	res = gf.Success().SetMsg("获取用户信息").SetData(data)
	return
}

// 修改用户信息
func (s *sAdminUser) SaveInfo(ctx context.Context, req *user.SaveInfoReq) (res *gf.R) {
	uid := ctx.Value("uid")
	if req.Type == "password" {
		security, err := accountsecurity.Default(ctx)
		if err != nil {
			return gf.Failed().SetMsg("账号安全服务暂不可用")
		}
		if err = security.ChangePassword(ctx, gconv.Int64(uid), gconv.Int64(uid), req.Oldpassword, req.Newpassword, "", false); err != nil {
			return gf.Failed().SetMsg("密码修改未通过，请检查当前密码和新密码")
		}
		_ = auth.RemoveToken(ctx, gconv.String(uid))
		imbridge.ClearHRMAssetCookie(ctx)
		return gf.Success().SetData(g.Map{"changed": true, "requiresLogin": true}).SetMsg("密码已修改，请重新登录")
	}
	var updata = make(gf.Map)
	if req.Type != "info" {
		if g.IsEmpty(req.Oldpassword) {
			res = gf.Failed().SetMsg("请输入旧密码")
			return
		}
		account, err := dao.Admin.Ctx(ctx).Where("id", uid).Fields("password,salt").One()
		if err != nil {
			res = gf.Failed().SetMsg("查找账号信息失败").SetData(err)
			return
		}
		salt := account["salt"].String()
		oldpass := gf.Md5(req.Oldpassword + salt)
		if oldpass != account["password"].String() {
			res = gf.Failed().SetMsg("输入的当前密码不正确！")
			return
		}
		if req.Type == "mobile" {
			code, emerr := gf.GetVerifyCode(req.Mobile)
			if emerr != nil || code != req.Captcha {
				res = gf.Failed().SetMsg("验证码无效").SetData(emerr)
				return
			}
			updata["mobile"] = req.Mobile
		} else if req.Type == "email" {
			code, emerr := gf.GetVerifyCode(req.Email)
			if emerr != nil || code != req.Captcha {
				res = gf.Failed().SetMsg("验证码无效").SetData(emerr)
				return
			}
			updata["email"] = req.Email
		} else if req.Type == "password" {
			updata["password"] = gf.Md5(req.Newpassword + salt)
			updata["pwd_reset_time"] = gtime.Datetime()
		}
		_, err = dao.Admin.Ctx(ctx).Where("id", uid).Data(updata).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新失败")
		} else {
			if req.Type == "password" {
				if revokeErr := imbridge.RevokeUID(ctx, gconv.Int64(uid)); revokeErr != nil {
					return gf.Failed().SetMsg("密码已更新，通讯会话撤销正在重试")
				}
			}
			res = gf.Success().SetMsg("更新成功！")
		}
	} else { //修改用户基础信息
		var upInfo = make(gf.Map)
		if !g.IsEmpty(req.Nickname) {
			upInfo["nickname"] = req.Nickname
		}
		if !g.IsEmpty(req.Avatar) {
			upInfo["avatar"] = req.Avatar
		}
		_, err := dao.Admin.Ctx(ctx).Where("id", uid).Update(upInfo)
		if err != nil {
			res = gf.Failed().SetMsg("更新失败")
		} else {
			res = gf.Success().SetMsg("更新成功！")
		}
	}
	return
}

// 退出登录
func (s *sAdminUser) LoginOut(ctx context.Context, req *user.LoginOutReq) (res *gf.R) {
	imbridge.ClearHRMAssetCookie(ctx)
	uid := ctx.Value("uid")
	if revokeErr := imbridge.RevokeUID(ctx, gconv.Int64(uid)); revokeErr != nil {
		return gf.Failed().SetMsg("通讯会话撤销正在重试，请稍后退出")
	}
	//删除缓存中的token，使其失效
	err := auth.RemoveToken(ctx, gconv.String(uid))
	if err != nil {
		res = gf.Failed().SetMsg("退出登录失败").SetData(err)
		return
	}
	res = gf.Success().SetData(uid).SetMsg("退出登录成功")
	return
}

// 获取管理后台菜单
func (s *sAdminUser) GetMenu(ctx context.Context, req *user.GetMenuReq) (res *gf.R) {
	uid := ctx.Value("uid")
	//1.获取用户授权角色组ID
	role_id, acerr := dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Array("role_id")
	if acerr != nil {
		res = gf.Failed().SetMsg("获取用户授权信息失败").SetData(acerr)
		return
	}
	if len(role_id) == 0 || role_id == nil {
		res = gf.Failed().SetMsg("您没有管理后台权限，请联系管理员授权")
		return
	}
	//2.查找用户所拥有的权限
	rule_ids, role_err := dao.AuthRole.Ctx(ctx).WhereIn("id", role_id).Where("status", 0).Array("rules")
	if role_err != nil {
		res = gf.Failed().SetMsg("查找用户role信息失败").SetData(role_err)
		return
	}
	//判断用户是否属于超级管理员角色
	isSuperAdmin, exerr := dao.AuthRole.Ctx(ctx).WhereIn("id", role_id).Where("status", 0).Where("rules", "*").Exist()
	if exerr != nil {
		res = gf.Failed().SetMsg("查询用户是否属于超级管理员失败").SetData(exerr)
		return
	}
	var roles []interface{}
	RMDB := dao.AuthRule.Ctx(ctx)
	if isSuperAdmin { //是超级角色
		roles = make([]interface{}, 0)
	} else { //不是超级权限-过滤菜单权限
		getmenus := gf.ArrayMerge(rule_ids)
		RMDB = RMDB.WhereIn("id", getmenus)
		roles = getmenus
	}
	nemu_list, ruleerr := RMDB.Where("status", 0).WhereIn("type", g.Slice{0, 1}).Order("weigh asc").All()
	if ruleerr != nil {
		res = gf.Failed().SetMsg("获取菜单权限错误").SetData(ruleerr)
		return
	}
	visibleMenus := middleware.FilterBizSuiteIMMenu(ctx, runtimeplugin.Default.FilterMenus(plugins.FilterMenus(nemu_list)))
	rulemenu := GetMenuArray(ctx, visibleMenus, 0, roles)
	res = gf.Success().SetMsg("获取管理后台菜单").SetData(rulemenu).SetExdata(roles)
	return
}

// 获取权限菜单
func GetMenuArray(ctx context.Context, pdata gdb.Result, parent_id int64, roles []interface{}) []map[string]interface{} {
	var returnList []map[string]interface{}
	var one int64 = 1
	for _, v := range pdata {
		if v["pid"].Int64() == parent_id {
			component := v["component"].String()
			if component == "PLUGIN:ebook" {
				component = "/album/" + v["routepath"].String() + "/index"
			}
			mid_item := map[string]interface{}{
				"path":      v["routepath"],
				"name":      v["routename"],
				"component": component,
			}
			children := GetMenuArray(ctx, pdata, v["id"].Int64(), roles)
			if children != nil {
				mid_item["children"] = children
			}
			//1.标题
			// var Menu_title interface{}
			// if v["locale"] != nil && v["locale"].String() != "" {
			// 	Menu_title = v["locale"]
			// } else {
			// 	Menu_title = v["title"]
			// }
			meta := map[string]interface{}{
				"locale": v["locale"],
				"title":  v["title"],
				"id":     v["id"],
			}
			//2.重定向
			if v["redirect"] != nil && v["redirect"].String() != "" {
				mid_item["redirect"] = v["redirect"]
			}
			//3.隐藏子菜单
			if v["hidechildreninmenu"] != nil && v["hidechildreninmenu"].Int64() == one {
				meta["hideChildrenInMenu"] = true
			}
			//3.图标
			if v["icon"] != nil && v["icon"].String() != "" {
				meta["icon"] = v["icon"]
			}
			//4.缓存
			if !v["keepalive"].IsEmpty() && v["keepalive"].Int64() == one { //设置为true页面将不会被缓存 false=缓存
				meta["ignoreCache"] = false
			} else {
				meta["ignoreCache"] = true
			}
			//5.隐藏菜单
			if v["hideinmenu"] != nil && v["hideinmenu"].Int64() == one {
				meta["hideInMenu"] = true
			}
			//6.在标签隐藏
			if v["noaffix"] != nil && v["noaffix"].Int64() == one {
				meta["noAffix"] = true
			}
			//7.详情页在本业打开-用于配置详情页时左侧激活的菜单路径
			if v["activemenu"] != nil && v["activemenu"].Int64() == one {
				meta["activeMenu"] = true
			}
			//8.是否需要登录鉴权
			if v["requiresauth"] != nil && v["requiresauth"].Int64() == one {
				meta["requiresAuth"] = true
			} else {
				meta["requiresAuth"] = false
			}
			//9.是否需要登录鉴权
			if v["isext"] != nil && v["isext"].Int64() == one {
				meta["isExt"] = true
			}
			//10.是否需要登录鉴权
			if v["onlypage"] != nil && v["onlypage"].Int64() == one {
				meta["onlypage"] = true
			} else {
				meta["onlypage"] = false
			}
			//11.按钮权限
			if len(roles) == 0 { //超级权限
				permission, _ := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).Where("pid", v["id"]).WhereNotNull("permission").Array("permission")
				if len(permission) > 0 {
					meta["btnroles"] = permission
				} else {
					meta["btnroles"] = [1]string{"*"}
				}
			} else { //选择路由
				permission, _ := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).Where("pid", v["id"]).WhereIn("id", roles).WhereNotNull("permission").Array("permission")
				if len(permission) > 0 {
					meta["btnroles"] = permission
				} else {
					hasepermission, _ := dao.AuthRule.Ctx(ctx).Where("status", 0).Where("type", 2).Where("pid", v["id"]).WhereNotNull("permission").Array("permission")
					if hasepermission == nil {
						meta["btnroles"] = make([]interface{}, 0)
					} else {
						meta["btnroles"] = [1]string{"*"}
					}
				}
			}
			//赋值
			if request := g.RequestFromCtx(ctx); request != nil && request.GetQuery("runtimeFrontend").String() != "1" {
				for prefix, plugin := range map[string]string{"album/": "ebook", "privatecode/": "privatecode", "analysis/": "analysis"} {
					if strings.HasPrefix(strings.TrimPrefix(component, "/"), prefix) && runtimeplugin.Default.Has(plugin) {
						mid_item["component"] = "systool/plugin/runtime"
						meta["runtimePlugin"] = plugin
					}
				}
			}
			mid_item["meta"] = meta
			returnList = append(returnList, mid_item)
		}
	}
	return returnList
}
