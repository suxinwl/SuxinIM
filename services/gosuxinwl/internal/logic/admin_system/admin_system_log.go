package adminsystem

import (
	"context"
	"encoding/json"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"github.com/suxinwl/GoSuxin/utility/plugin"
	"net"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// 1.1获取账号数据
func (s *sAdminsystem) GetLogin(ctx context.Context, req *system.GetLoginReq) (res *gf.R) {
	//组合搜索条件
	whereMap := gmap.New()
	if !g.IsEmpty(req.User) {
		userids, _ := dao.Admin.Ctx(ctx).Where("name like ?", "%"+req.User+"%").Array("id")
		whereMap.Set("uid IN(?)", userids)
	}
	if !g.IsEmpty(req.Ip) {
		address := net.ParseIP(req.Ip)
		if address == nil {
			whereMap.Set("address like ?", "%"+req.Ip+"%")
		} else {
			whereMap.Set("ip", req.Ip)
		}
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status =?", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	MDB := dao.LoginLog.Ctx(ctx).Where(whereMap)
	totalCount, _ := MDB.Clone().Count()
	list, err := MDB.Page(req.Page, req.PageSize).Order("id desc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取登录日志失败").SetData(err)
		return
	}
	for _, val := range list {
		userdata, _ := dao.Admin.Ctx(ctx).Where("id", val["uid"]).Fields("username,name,nickname,avatar").One()
		val["user"] = gvar.New(userdata)
	}
	res = gf.Success().SetMsg("获取登录日志列表").SetData(gf.Map{
		"page":     req.Page,
		"pageSize": req.PageSize,
		"total":    totalCount,
		"items":    list})
	return
}

// 1.2删除上个月登录日志
func (s *sAdminsystem) DelLastLogin(ctx context.Context, in *system.DelLastLoginReq) (res *gf.R) {
	result, err := dao.LoginLog.Ctx(ctx).Where("createtime <", gtime.Now().AddDate(0, -1, 0).Format("Y-m-d H:i:s")).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("删除成功！").SetData(result)
	return
}

// 2.1获取操作日志列表
func (s *sAdminsystem) GetOperation(ctx context.Context, req *system.GetOperationReq) (res *gf.R) {
	//组合搜索条件
	whereMap := gmap.New()
	if !g.IsEmpty(req.User) {
		userids, _ := dao.Admin.Ctx(ctx).Where("name like ?", "%"+req.User+"%").Array("id")
		whereMap.Set("uid IN(?)", userids)
	}
	if !g.IsEmpty(req.Ip) {
		address := net.ParseIP(req.Ip)
		if address == nil {
			whereMap.Set("address like ?", "%"+req.Ip+"%")
		} else {
			whereMap.Set("ip", req.Ip)
		}
	}
	if !g.IsEmpty(req.Status) {
		whereMap.Set("status ?", req.Status)
	}
	if times := gf.FindTimeCondition(req.Createtime); times != nil {
		whereMap.Set("createtime between ? and ?", times)
	}
	MDB := dao.OperationLog.Ctx(ctx).Where(whereMap)
	totalCount, _ := MDB.Clone().Count()
	list, err := MDB.Page(req.Page, req.PageSize).Order("id desc").All()
	if err != nil {
		res = gf.Failed().SetMsg("获取操作日志失败").SetData(err)
		return
	}
	for _, val := range list {
		userdata, _ := dao.Admin.Ctx(ctx).Where("id", val["uid"]).Fields("username,name,nickname,avatar").One()
		val["user"] = gvar.New(userdata)
	}
	res = gf.Success().SetMsg("获取操作日志列表").SetData(gf.Map{
		"page":     req.Page,
		"pageSize": req.PageSize,
		"total":    totalCount,
		"items":    list})
	return
}

// 2.2删除上个月操作日志
func (s *sAdminsystem) DelLastOperation(ctx context.Context, in *system.DelLastOperationReq) (res *gf.R) {
	result, err := dao.OperationLog.Ctx(ctx).Where("createtime <", gtime.Now().AddDate(0, -1, 0).Format("Y-m-d H:i:s")).Delete()
	if err != nil {
		res = gf.Failed().SetMsg("删除失败").SetData(err)
		return
	}
	res = gf.Success().SetMsg("删除成功！").SetData(result)
	return
}

// 2.3获取操作日志内容
func (s *sAdminsystem) GetOperationDetail(ctx context.Context, req *system.GetOperationDetailReq) (res *gf.R) {
	data, err := dao.OperationLog.Ctx(ctx).Where("id", req.Id).One()
	if err != nil {
		res = gf.Failed().SetMsg("获取内容失败").SetData(err)
	} else {
		if data != nil {
			data["username"], _ = dao.Admin.Ctx(ctx).Where("id", data["uid"]).Value("name")
		}
		res = gf.Success().SetMsg("获取内容成功！").SetData(data)
	}
	return
}

// 忽略操作日志请求接口
var (
	noSaveUrl = []string{"/admin/system/log"}
)

// 2.4 操作日志写入
func (s *sAdminsystem) OperationLog(r *ghttp.Request) {
	r.Middleware.Next()
	uid := r.GetCtxVar("uid")
	if uid.IsEmpty() {
		return
	}
	// 忽略操作日志请求接口
	if GetNoInUrls(r.URL.Path) {
		return
	}
	ctx := r.Context()
	ip := r.GetClientIp()
	if ip == "::1" {
		ip = "127.0.0.1"
	}
	address, _ := plugin.NewIpRegion(ip)
	//查找菜单接口名称
	des_str := ""
	authdata, err := dao.AuthRule.Ctx(ctx).Where("path", r.URL.Path).Fields("pid,title,des").One()
	if err == nil && !authdata.IsEmpty() {
		ftitle, _ := dao.AuthRule.Ctx(ctx).Where("id", authdata["pid"]).Value("title")
		substr := authdata["des"].String()
		if authdata["des"].IsEmpty() {
			substr = authdata["title"].String()
		}
		des_str = ftitle.String() + "【" + substr + "】"
	}
	reqBody, respBody := operationLogBodies(r.URL.Path, func() (any, any) {
		//处理请求参数
		dataMap := make(map[string]interface{})
		//1.post的body参数
		bodyparam := r.GetBody()
		var parameter map[string]interface{}
		if err := json.Unmarshal(bodyparam, &parameter); err == nil {
			dataMap = parameter
		}
		//2.url参数
		urlparam := r.GetQueryMap()
		if len(urlparam) > 0 {
			delete(urlparam, "_t")
		}
		if len(urlparam) > 0 {
			for key, val := range urlparam {
				dataMap[key] = val
			}
		}
		// Storage secrets must never be persisted by the generic request logger.
		for key := range dataMap {
			switch strings.ToLower(key) {
			case "clientsecret", "cdnkey", "secret", "password", "oldpassword", "newpassword", "salt", "changeticket", "sessiontoken", "token", "authorization":
				dataMap[key] = "[REDACTED]"
			}
		}
		return dataMap, r.GetHandlerResponse()
	})
	savedata := gf.Map{
		"uid":          uid,
		"method":       r.Method,
		"url":          r.URL.Path,
		"ip":           ip,
		"address":      address,
		"des":          des_str,
		"status":       r.Response.Status,
		"req_headers":  redactedLogHeaders(r.Request.Header),
		"req_body":     reqBody,
		"resp_body":    respBody,
		"resp_headers": redactedLogHeaders(r.Response.Header()),
		"latency":      0,
	}
	dao.OperationLog.Ctx(ctx).Insert(savedata)
}

// 过滤忽略请求
func GetNoInUrls(url string) bool {
	for _, val := range noSaveUrl {
		if strings.HasPrefix(url, val) {
			return true
		}
	}
	return false
}
