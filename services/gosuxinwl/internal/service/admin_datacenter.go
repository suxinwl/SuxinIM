// ================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// You can delete these comments if you wish manually maintain this interface file.
// ================================================================================

package service

import (
	"context"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type (
	IAdmindatacenter interface {
		// 获取应用配置数据
		GetConfig(ctx context.Context, req *datacenter.GetConfigReq) (res *gf.R)
		// 保存应用配置数据
		SaveConfig(ctx context.Context, req *datacenter.SaveConfigReq) (res *gf.R)
		// 获取我的附件
		GetMyFiles(ctx context.Context, req *datacenter.GetMyFilesReq) (res *gf.R)
		// 新建文件夹
		Save(ctx context.Context, req *datacenter.SaveReq) (res *gf.R)
		// 删除文件夹
		DelDir(ctx context.Context, req *datacenter.DelDirReq) (res *gf.R)
		Del(ctx context.Context, req *datacenter.DelReq) (res *gf.R)
		// 移动文件到文件夹
		UpImgPid(ctx context.Context, req *datacenter.UpImgPidReq) (res *gf.R)
		// 获取邮箱数据
		GetEmail(ctx context.Context, req *datacenter.GetEmailReq) (res *gf.R)
		// 保存邮箱数据
		SaveEmail(ctx context.Context, req *datacenter.SaveEmailReq) (res *gf.R)
		// 获取安装的代码仓配置
		GetCodestoreConfig(ctx context.Context, req *datacenter.GetCodestoreConfigReq) (res *gf.R)
		// 修改安装的代码仓配置
		SaveCodeStoreConfig(ctx context.Context, req *datacenter.SaveCodeStoreConfigReq) (res *gf.R)
		// 更新配置使用状态
		UpConfigStatus(ctx context.Context, req *datacenter.UpConfigStatusReq) (res *gf.R)
		// 获取字典数据
		DictionaryList(ctx context.Context, req *datacenter.DictionaryListReq) (res *gf.R)
		// 保存、编辑字典数据
		DictionarySave(ctx context.Context, req *datacenter.DictionarySaveReq) (res *gf.R)
		// 更新状态
		DictionaryStatus(ctx context.Context, req *datacenter.DictionaryStatusReq) (res *gf.R)
		// 删除字典数据
		DictionaryDel(ctx context.Context, req *datacenter.DictionaryDelReq) (res *gf.R)
		// 使用数据表数据(表单生成使用)
		GetTableDataForm(ctx context.Context, req *datacenter.GetTableDataFormReq) (res *gf.R)
		// 获取字典分类数据
		TabledataList(ctx context.Context, req *datacenter.TabledataListReq) (res *gf.R)
		// 保存、编辑字典分组
		TabledataSave(ctx context.Context, req *datacenter.TabledataSaveReq) (res *gf.R)
		// 删除字典分组
		TabledataDel(ctx context.Context, req *datacenter.TabledataDelReq) (res *gf.R)
		// 管理后台上传附件接口
		Upload(ctx context.Context, req *datacenter.UploadReq) (res *gf.R)
		GetUploadconfig(ctx context.Context, req *datacenter.GetUploadconfigReq) *gf.R
		SaveUploadconfig(ctx context.Context, req *datacenter.SaveUploadconfigReq) *gf.R
		TestConnection(ctx context.Context, req *datacenter.TestConnectionReq) *gf.R
	}
)

var (
	localAdmindatacenter IAdmindatacenter
)

func Admindatacenter() IAdmindatacenter {
	if localAdmindatacenter == nil {
		panic("implement not found for interface IAdmindatacenter, forgot register?")
	}
	return localAdmindatacenter
}

func RegisterAdmindatacenter(i IAdmindatacenter) {
	localAdmindatacenter = i
}
