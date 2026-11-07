package admindatacenter

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/utility/extend/uploads"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/container/gmap"
	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取我的附件
func (s *sAdmindatacenter) GetMyFiles(ctx context.Context, req *datacenter.GetMyFilesReq) (res *gf.R) {
	//组合搜索条件
	whereMap := gmap.New()
	whereMap.Set("pid", req.Pid)
	whereMap.Set("is_common", 0)
	if !g.IsEmpty(req.Searchword) {
		whereMap.Set("title like ?", "%"+req.Searchword+"%")
	}
	if req.Filetype == "video" {
		whereMap.Set("type IN(?)", []interface{}{1, 2})
	} else if req.Filetype == "audio" {
		whereMap.Set("type IN(?)", []interface{}{1, 3})
	} else if req.Filetype == "file" {
		whereMap.Set("type IN(?)", []interface{}{1, 4})
	} else { //默认图片
		whereMap.Set("type IN(?)", []interface{}{0, 1})
	}
	list, err := dao.Attachment.Ctx(ctx).Where(whereMap).Fields("id,pid,name,title,type,url,filesize,mimetype,storage,cover_url,is_common").Order("type desc,weigh desc,id desc").All()
	if err != nil {
		res = gf.Failed().SetMsg("加载数据失败").SetData(err)
		return
	}
	rooturl := ""
	for _, val := range list {
		if _, ok := val["cover_url"]; ok && val["cover_url"].String() != "" && !strings.Contains(val["cover_url"].String(), "http") && rooturl != "" {
			val["cover_url"] = gvar.New(rooturl + val["cover_url"].String())
		}
	}
	common_lisr, _ := dao.Attachment.Ctx(ctx).Where("pid", req.Pid).Where("is_common", 1).Fields("id,pid,name,title,type,url,filesize,mimetype,storage,cover_url,is_common").Order("type desc,weigh desc,id desc").All()
	if list != nil {
		list = append(common_lisr, list...)
	} else {
		list = common_lisr
	}
	var totalCount int64
	//获取目录菜单
	allids := clogic.GetAllParentIds(ctx, req.Pid)
	allids = append(allids, req.Pid)
	dirmenu, _ := dao.Attachment.Ctx(ctx).WhereIn("id", allids).Fields("id,pid,title").All()
	if dirmenu.IsEmpty() {
		dirmenu = make(gdb.Result, 0)
	}
	hasegallery := false
	resdata := gf.Map{
		"total":       totalCount,
		"dirmenu":     dirmenu,
		"allids":      allids,
		"items":       list,
		"hasegallery": hasegallery, //是否存在图库
	}
	res = gf.Success().SetMsg("获取附件列表").SetData(resdata)
	return
}

// 新建文件夹
func (s *sAdmindatacenter) Save(ctx context.Context, req *datacenter.SaveReq) (res *gf.R) {
	if req.Id == gf.ZeroValue {
		fileCount, _ := dao.Attachment.Ctx(ctx).Where("pid", req.Pid).Where("title like ?", fmt.Sprintf("%s%v%s", "%", req.Title, "%")).Count()
		req.Title = fmt.Sprintf("%s%v", req.Title, fileCount+1)
		data := g.Map{"pid": req.Pid, "title": attachmentLabel(req.Title), "name": attachmentLabel(req.Title), "type": req.Type, "url": "", "imagewidth": "", "imageheight": "", "mimetype": "", "extparam": "", "cover_url": "", "sha1": ""}
		addId, err := dao.Attachment.Ctx(ctx).Data(data).InsertAndGetId()
		if err != nil {
			res = gf.Failed().SetMsg("添加失败").SetData(err)
		} else {
			dao.Attachment.Ctx(ctx).Where("id", addId).Update(map[string]interface{}{"weigh": addId})
			getdata, _ := dao.Attachment.Ctx(ctx).Where("id", addId).Fields("id,pid,name,title,type,url,filesize,mimetype,storage").All()
			res = gf.Success().SetMsg("添加成功").SetData(getdata)
		}
	} else { //更新文件名称
		result, err := dao.Attachment.Ctx(ctx).Data(gf.Map{"title": req.Title}).Where("id", req.Id).Update()
		if err != nil {
			res = gf.Failed().SetMsg("更新失败").SetData(err)
		} else {
			res = gf.Success().SetMsg("更新成功！").SetData(result)
		}
	}
	return
}

// 删除文件夹
func (s *sAdmindatacenter) DelDir(ctx context.Context, req *datacenter.DelDirReq) (res *gf.R) {
	uploadConfigMu.RLock()
	defer uploadConfigMu.RUnlock()
	if err := deleteAttachmentTree(ctx, req.Id, map[string]bool{}); err != nil {
		return gf.Failed().SetMsg("删除未完成，失败文件及其目录已保留：" + err.Error())
	}
	return gf.Success().SetMsg("删除文件夹成功").SetData(true)
}
func (s *sAdmindatacenter) Del(ctx context.Context, req *datacenter.DelReq) (res *gf.R) {
	uploadConfigMu.RLock()
	defer uploadConfigMu.RUnlock()
	for _, id := range req.Ids {
		if err := deleteAttachmentTree(ctx, id, map[string]bool{}); err != nil {
			return gf.Failed().SetMsg("删除未完成，失败文件已保留：" + err.Error())
		}
	}
	return gf.Success().SetMsg("删除成功").SetData(true)
}
func deleteAttachmentTree(ctx context.Context, id interface{}, visited map[string]bool) error {
	key := fmt.Sprint(id)
	if visited[key] {
		return fmt.Errorf("附件目录存在循环引用")
	}
	visited[key] = true
	row, e := dao.Attachment.Ctx(ctx).Where("id", id).One()
	if e != nil {
		return e
	}
	if row.IsEmpty() {
		return nil
	}
	if row["type"].Int() == 1 {
		children, e := dao.Attachment.Ctx(ctx).Where("pid", id).Array("id")
		if e != nil {
			return e
		}
		for _, child := range children {
			if e = deleteAttachmentTree(ctx, child, visited); e != nil {
				return e
			}
		}
	} else if row["url"].String() != "" {
		e = uploads.WithContext(ctx, row["location"].String()).RemoveFile(row["url"].String())
		if e != nil {
			return e
		}
	}
	_, e = dao.Attachment.Ctx(ctx).Where("id", id).Delete()
	return e
}

// 移动文件到文件夹
func (s *sAdmindatacenter) UpImgPid(ctx context.Context, req *datacenter.UpImgPidReq) (res *gf.R) {
	result, err := dao.Attachment.Ctx(ctx).Where("id", req.Imgid).Data(gf.Map{"pid": req.Pid}).Update()
	if err != nil {
		res = gf.Failed().SetMsg("更新失败").SetData(err)
	} else {
		res = gf.Success().SetMsg("更新成功！").SetData(result)
	}
	return
}
