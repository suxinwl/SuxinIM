package admindatacenter

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/extend/uploads"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"io"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 管理后台上传附件接口
func (s *sAdmindatacenter) Upload(ctx context.Context, req *datacenter.UploadReq) (res *gf.R) {
	uploadConfigMu.RLock()
	defer uploadConfigMu.RUnlock()
	if req.File == nil {
		res = gf.Failed().SetMsg("no file uploaded")
		return
	}
	AllowedExt, _ := g.Cfg("upload").Get(ctx, "AllowedExt")
	AllowedExt_arr := strings.Split(AllowedExt.String(), ",")
	ext := strings.ToLower(path.Ext(req.File.Filename))
	if !slices.Contains(AllowedExt_arr, ext) {
		res = gf.Failed().SetMsg("上传不支持" + ext + "的文件类型")
		return
	}
	//判断文件是否已经传过
	fileContent, openErr := req.File.Open()
	if openErr != nil {
		return gf.Failed().SetMsg("读取上传文件失败")
	}
	m_d5 := md5.New()
	size, hashErr := io.Copy(m_d5, fileContent)
	fileContent.Close()
	if hashErr != nil {
		return gf.Failed().SetMsg("计算文件摘要失败")
	}
	maxSize, _ := g.Cfg("upload").Get(ctx, "MaxBodySize")
	if size > maxSize.Int64()*1024*1024 {
		return gf.Failed().SetMsg("文件超过上传大小限制")
	}
	sha1_str := hex.EncodeToString(m_d5.Sum(nil))
	location, configErr := g.Cfg("upload").Get(ctx, "Type")
	if configErr != nil {
		return gf.Failed().SetMsg("读取存储配置失败")
	}
	//查找该用户是否传过
	attachment, lookupErr := dao.Attachment.Ctx(ctx).Where("sha1", sha1_str).Where("location", location.String()).Fields("id,pid,name,title,type,url,filesize,mimetype,cover_url as cover").One()
	if lookupErr != nil {
		return gf.Failed().SetMsg("检查重复附件失败")
	}
	if !attachment.IsEmpty() { //文件经存在，则直接返回
		//更新排序到最前面
		maxId, _ := dao.Attachment.Ctx(ctx).Order("weigh desc").Value("id")
		if maxId != nil {
			dao.Attachment.Ctx(ctx).Data(gf.Map{"weigh": maxId.Int() + 1, "pid": req.Pid}).Where("id", attachment["id"]).Update()
		}
		attachment["url"].Set(gf.GetFullUrl(attachment["url"].String()))
		res = gf.Success().SetMsg("文件已上传").SetData(attachment)
		return
	}
	//文件类型
	var ftype int64 = 0
	switch req.Filetype {
	case "video": //视频
		ftype = 2
	case "audio": //音频
		ftype = 3
	case "file": //附件类
		ftype = 4
	case "image": //图片
		ftype = 0
	default:
		ftype = 5
	}
	filename_arr := strings.Split(req.File.Filename, ".")
	//处理文件上传，bin返回地址
	provider := uploads.WithContext(ctx, location.String())
	url, cover_url, err := provider.UploadFile(req.File)
	if err != nil {
		res = gf.Failed().SetMsg("上传文件失败：" + err.Error())
		return
	}
	fileData := gf.Map{
		"imagewidth":  "",
		"imageheight": "",
		"extparam":    "",
		"storage":     location.String(),
		"type":        ftype,
		"location":    location, //存储位置
		"pid":         req.Pid,
		"sha1":        sha1_str, //文件唯一指纹
		"name":        attachmentLabel(req.File.Filename),
		"title":       attachmentLabel(filename_arr[0]),
		"url":         url,       //附件路径
		"cover_url":   cover_url, //封面
		"filesize":    req.File.Size,
		"mimetype":    req.File.Header.Get("Content-Type"),
	}
	//保存到数据库
	addId, err := dao.Attachment.Ctx(ctx).Data(fileData).InsertAndGetId()
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cleanupErr := uploads.WithContext(cleanup, location.String()).RemoveFile(url)
		if cleanupErr != nil {
			return gf.Failed().SetMsg("保存附件记录失败，云端清理未完成").SetData(cleanupErr.Error())
		}
		res = gf.Failed().SetMsg("保存上传数据失败").SetData(err)
		return
	}
	fileData["id"] = addId
	//处理预览url地址
	fileData["url"] = gf.GetFullUrl(url)
	res = gf.Success().SetMsg("文件上传成功").SetData(fileData)
	return
}

func attachmentLabel(value string) string {
	chars := []rune(value)
	if len(chars) > 50 {
		return string(chars[:50])
	}
	return value
}
