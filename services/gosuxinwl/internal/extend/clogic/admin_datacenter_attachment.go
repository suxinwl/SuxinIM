package clogic

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// 工具
func GetAllParentIds(ctx context.Context, id interface{}) []interface{} {
	var parent_ids []interface{}
	parent_id, _ := dao.Attachment.Ctx(ctx).Where("id", id).Value("pid")
	if parent_id != nil {
		parent_ids = append(parent_ids, parent_id)
		parent_ids = append(parent_ids, GetAllParentIds(ctx, parent_id)...)
	}
	return parent_ids
}

// 循环删除文件夹及文件
func DelCycle(ctx context.Context, id interface{}) {
	filedata, _ := dao.Attachment.Ctx(ctx).Where("pid", id).Fields("id,type").All()
	if !filedata.IsEmpty() && len(filedata) > 0 {
		for _, val := range filedata {
			DelCycle(ctx, val["id"])
			if val["type"].Int64() == 0 {
				file_path, _ := dao.Attachment.Ctx(ctx).Where("id", val["id"]).Value("url")
				if file_path != nil {
					gf.DelOneFile(file_path.String())
				}
			}
			dao.Attachment.Ctx(ctx).Where("id", val["id"]).Delete()
		}
	}
}
