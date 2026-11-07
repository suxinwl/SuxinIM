package admindatacenter

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/extend/pan123"
	"github.com/suxinwl/GoSuxin/utility/extend/uploadconfig"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"sync"
	"time"
)

var uploadConfigMu sync.RWMutex

func (s *sAdmindatacenter) GetUploadconfig(ctx context.Context, req *datacenter.GetUploadconfigReq) *gf.R {
	data := g.Map{}
	for _, key := range []string{"Type", "MaxBodySize", "AllowedExt", "local", "alioss", "tencentcos", "qiniuoss"} {
		v, e := g.Cfg("upload").Get(ctx, key)
		if e != nil {
			return gf.Failed().SetMsg("读取上传配置失败")
		}
		data[key] = v
	}
	p, e := pan123.Load(ctx)
	if e != nil {
		return gf.Failed().SetMsg("读取123云盘配置失败")
	}
	data["pan123"] = g.Map{"clientID": p.ClientID, "parentId": p.ParentID, "urlAuth": p.URLAuthEnabled(), "uid": p.UID, "hasClientSecret": p.ClientSecret != "", "hasCDNKey": p.CDNKey != ""}
	return gf.Success().SetMsg("获取文件上传配置").SetData(data)
}
func panCandidate(ctx context.Context, in datacenter.Pan123Fields) (pan123.Config, error) {
	old, e := pan123.Load(ctx)
	if e != nil {
		return old, e
	}
	p := pan123.Config{ClientID: in.ClientID, ClientSecret: in.ClientSecret, ParentID: in.ParentID, URLAuth: in.URLAuth, CDNKey: in.CDNKey}
	return pan123.Merge(old, p)
}
func (s *sAdmindatacenter) SaveUploadconfig(ctx context.Context, req *datacenter.SaveUploadconfigReq) *gf.R {
	uploadConfigMu.Lock()
	defer uploadConfigMu.Unlock()
	if req.MaxBodySize <= 0 {
		return gf.Failed().SetMsg("上传大小必须大于零")
	}
	root := g.Map{"Type": req.Type, "MaxBodySize": req.MaxBodySize, "AllowedExt": req.AllowedExt}
	var fields g.Map
	switch req.Type {
	case "local":
		fields = g.Map{"LBaseUrl": req.BaseUrl, "LDirPath": req.DirPath}
	case "alioss":
		fields = g.Map{"ABaseUrl": req.BaseUrl, "AEndpoint": req.Endpoint, "AKeyId": req.KeyId, "ASecret": req.Secret, "ABucketName": req.BucketName, "ADirPath": req.DirPath}
	case "tencentcos":
		fields = g.Map{"TBaseUrl": req.BaseUrl, "TEndpoint": req.Endpoint, "TKeyId": req.KeyId, "TSecret": req.Secret, "TBucketName": req.BucketName, "TRegion": req.Region, "TDirPath": req.DirPath}
	case "qiniuoss":
		fields = g.Map{"QBaseUrl": req.BaseUrl, "QEndpoint": req.Endpoint, "QKeyId": req.KeyId, "QSecret": req.Secret, "QBucketName": req.BucketName, "QDestBucketName": req.DestBucketName, "QDirPath": req.DirPath, "QUseHTTPS": req.UseHTTPS, "QZone": req.Zone}
	case "pan123":
		p, e := panCandidate(ctx, req.Pan123Fields)
		if e != nil {
			return gf.Failed().SetMsg(e.Error())
		}
		check, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		uid, e := pan123.ValidateAccount(check, p)
		if e != nil {
			return gf.Failed().SetMsg(e.Error())
		}
		old, e := pan123.Load(ctx)
		if e != nil {
			return gf.Failed().SetMsg(e.Error())
		}
		count, e := dao.Attachment.Ctx(ctx).Where("location", "pan123").Count()
		if e != nil {
			return gf.Failed().SetMsg("检查旧附件失败")
		}
		if count > 0 && old.UID != uid {
			return gf.Failed().SetMsg("已有123云盘附件，不能更换为不同账号")
		}
		fields = g.Map{"clientID": p.ClientID, "clientSecret": p.ClientSecret, "parentId": p.ParentID, "urlAuth": p.URLAuthEnabled(), "cdnKey": p.CDNKey, "uid": uid}
	default:
		return gf.Failed().SetMsg("不支持的存储类型")
	}
	if e := uploadconfig.Save("manifest/config/upload.yaml", root, req.Type, fields); e != nil {
		return gf.Failed().SetMsg("保存文件上传配置失败").SetData(e.Error())
	}
	return gf.Success().SetMsg("保存文件上传配置成功").SetData(true)
}
func (s *sAdmindatacenter) TestConnection(ctx context.Context, req *datacenter.TestConnectionReq) *gf.R {
	p, e := panCandidate(ctx, req.Pan123Fields)
	if e != nil {
		return gf.Failed().SetMsg(e.Error())
	}
	steps, uid, e := pan123.Test(ctx, p)
	return gf.Success().SetMsg("连接测试完成").SetData(g.Map{"steps": steps, "uid": uid, "success": e == nil})
}
