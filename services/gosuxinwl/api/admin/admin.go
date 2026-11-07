// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package admin

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/admin/common"
	"github.com/suxinwl/GoSuxin/api/admin/dashboard"
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/api/admin/install"
	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/api/admin/user"
)

type IAdminCommon interface {
	MessageList(ctx context.Context, req *common.MessageListReq) (res *common.MessageListRes, err error)
}

type IAdminDashboard interface {
	GetQuick(ctx context.Context, req *dashboard.GetQuickReq) (res *dashboard.GetQuickRes, err error)
	SaveQuick(ctx context.Context, req *dashboard.SaveQuickReq) (res *dashboard.SaveQuickRes, err error)
}

type IAdminDatacenter interface {
	GetConfig(ctx context.Context, req *datacenter.GetConfigReq) (res *datacenter.GetConfigRes, err error)
	SaveConfig(ctx context.Context, req *datacenter.SaveConfigReq) (res *datacenter.SaveConfigRes, err error)
	GetMyFiles(ctx context.Context, req *datacenter.GetMyFilesReq) (res *datacenter.GetMyFilesRes, err error)
	Save(ctx context.Context, req *datacenter.SaveReq) (res *datacenter.SaveRes, err error)
	DelDir(ctx context.Context, req *datacenter.DelDirReq) (res *datacenter.DelDirRes, err error)
	Del(ctx context.Context, req *datacenter.DelReq) (res *datacenter.DelRes, err error)
	UpImgPid(ctx context.Context, req *datacenter.UpImgPidReq) (res *datacenter.UpImgPidRes, err error)
	GetEmail(ctx context.Context, req *datacenter.GetEmailReq) (res *datacenter.GetEmailRes, err error)
	SaveEmail(ctx context.Context, req *datacenter.SaveEmailReq) (res *datacenter.SaveEmailRes, err error)
	GetCodestoreConfig(ctx context.Context, req *datacenter.GetCodestoreConfigReq) (res *datacenter.GetCodestoreConfigRes, err error)
	SaveCodeStoreConfig(ctx context.Context, req *datacenter.SaveCodeStoreConfigReq) (res *datacenter.SaveCodeStoreConfigRes, err error)
	UpConfigStatus(ctx context.Context, req *datacenter.UpConfigStatusReq) (res *datacenter.UpConfigStatusRes, err error)
	DictionaryList(ctx context.Context, req *datacenter.DictionaryListReq) (res *datacenter.DictionaryListRes, err error)
	DictionarySave(ctx context.Context, req *datacenter.DictionarySaveReq) (res *datacenter.DictionarySaveRes, err error)
	DictionaryStatus(ctx context.Context, req *datacenter.DictionaryStatusReq) (res *datacenter.DictionaryStatusRes, err error)
	DictionaryDel(ctx context.Context, req *datacenter.DictionaryDelReq) (res *datacenter.DictionaryDelRes, err error)
	GetTableDataForm(ctx context.Context, req *datacenter.GetTableDataFormReq) (res *datacenter.GetTableDataFormRes, err error)
	TabledataList(ctx context.Context, req *datacenter.TabledataListReq) (res *datacenter.TabledataListRes, err error)
	TabledataSave(ctx context.Context, req *datacenter.TabledataSaveReq) (res *datacenter.TabledataSaveRes, err error)
	TabledataDel(ctx context.Context, req *datacenter.TabledataDelReq) (res *datacenter.TabledataDelRes, err error)
	Upload(ctx context.Context, req *datacenter.UploadReq) (res *datacenter.UploadRes, err error)
	GetUploadconfig(ctx context.Context, req *datacenter.GetUploadconfigReq) (res *datacenter.GetUploadconfigRes, err error)
	SaveUploadconfig(ctx context.Context, req *datacenter.SaveUploadconfigReq) (res *datacenter.SaveUploadconfigRes, err error)
	TestConnection(ctx context.Context, req *datacenter.TestConnectionReq) (res *datacenter.TestConnectionRes, err error)
}

type IAdminDeveloper interface {
	GetCodeCate(ctx context.Context, req *developer.GetCodeCateReq) (res *developer.GetCodeCateRes, err error)
	CodeList(ctx context.Context, req *developer.CodeListReq) (res *developer.CodeListRes, err error)
	Login(ctx context.Context, req *developer.LoginReq) (res *developer.LoginRes, err error)
	AsyncVersion(ctx context.Context, req *developer.AsyncVersionReq) (res *developer.AsyncVersionRes, err error)
	UpPrivateHouse(ctx context.Context, req *developer.UpPrivateHouseReq) (res *developer.UpPrivateHouseRes, err error)
	CheckPackName(ctx context.Context, req *developer.CheckPackNameReq) (res *developer.CheckPackNameRes, err error)
	SavePackName(ctx context.Context, req *developer.SavePackNameReq) (res *developer.SavePackNameRes, err error)
	Requirement(ctx context.Context, req *developer.RequirementReq) (res *developer.RequirementRes, err error)
	UpPackToService(ctx context.Context, req *developer.UpPackToServiceReq) (res *developer.UpPackToServiceRes, err error)
	GetPackdirs(ctx context.Context, req *developer.GetPackdirsReq) (res *developer.GetPackdirsRes, err error)
	GetMenutree(ctx context.Context, req *developer.GetMenutreeReq) (res *developer.GetMenutreeRes, err error)
	MenuTreeToJson(ctx context.Context, req *developer.MenuTreeToJsonReq) (res *developer.MenuTreeToJsonRes, err error)
	LoginCode(ctx context.Context, req *developer.LoginCodeReq) (res *developer.LoginCodeRes, err error)
	FreeLogin(ctx context.Context, req *developer.FreeLoginReq) (res *developer.FreeLoginRes, err error)
	RegisterUser(ctx context.Context, req *developer.RegisterUserReq) (res *developer.RegisterUserRes, err error)
	PackCode(ctx context.Context, req *developer.PackCodeReq) (res *developer.PackCodeRes, err error)
	Upfile(ctx context.Context, req *developer.UpfileReq) (res *developer.UpfileRes, err error)
	DownCode(ctx context.Context, req *developer.DownCodeReq) (res *developer.DownCodeRes, err error)
	InstallCode(ctx context.Context, req *developer.InstallCodeReq) (res *developer.InstallCodeRes, err error)
	UninstallCode(ctx context.Context, req *developer.UninstallCodeReq) (res *developer.UninstallCodeRes, err error)
	InstallLocalCode(ctx context.Context, req *developer.InstallLocalCodeReq) (res *developer.InstallLocalCodeRes, err error)
	GetInstallPack(ctx context.Context, req *developer.GetInstallPackReq) (res *developer.GetInstallPackRes, err error)
}

type IAdminInstall interface {
	GetConfig(ctx context.Context, req *install.GetConfigReq) (res *install.GetConfigRes, err error)
	Save(ctx context.Context, req *install.SaveReq) (res *install.SaveRes, err error)
}

type IAdminSystem interface {
	AccountList(ctx context.Context, req *system.AccountListReq) (res *system.AccountListRes, err error)
	Isaccountexist(ctx context.Context, req *system.IsaccountexistReq) (res *system.IsaccountexistRes, err error)
	AccountSave(ctx context.Context, req *system.AccountSaveReq) (res *system.AccountSaveRes, err error)
	AccountStatus(ctx context.Context, req *system.AccountStatusReq) (res *system.AccountStatusRes, err error)
	AccountDel(ctx context.Context, req *system.AccountDelReq) (res *system.AccountDelRes, err error)
	DeptList(ctx context.Context, req *system.DeptListReq) (res *system.DeptListRes, err error)
	DeptParent(ctx context.Context, req *system.DeptParentReq) (res *system.DeptParentRes, err error)
	DeptSave(ctx context.Context, req *system.DeptSaveReq) (res *system.DeptSaveRes, err error)
	DeptLeaderOptions(ctx context.Context, req *system.DeptLeaderOptionsReq) (*system.DeptLeaderOptionsRes, error)
	DeptIMGroup(ctx context.Context, req *system.DeptIMGroupReq) (*system.DeptIMGroupRes, error)
	DeptStatus(ctx context.Context, req *system.DeptStatusReq) (res *system.DeptStatusRes, err error)
	DeptDel(ctx context.Context, req *system.DeptDelReq) (res *system.DeptDelRes, err error)
	DragDept(ctx context.Context, req *system.DragDeptReq) (res *system.DragDeptRes, err error)
	GetLogin(ctx context.Context, req *system.GetLoginReq) (res *system.GetLoginRes, err error)
	DelLastLogin(ctx context.Context, req *system.DelLastLoginReq) (res *system.DelLastLoginRes, err error)
	GetOperation(ctx context.Context, req *system.GetOperationReq) (res *system.GetOperationRes, err error)
	DelLastOperation(ctx context.Context, req *system.DelLastOperationReq) (res *system.DelLastOperationRes, err error)
	GetOperationDetail(ctx context.Context, req *system.GetOperationDetailReq) (res *system.GetOperationDetailRes, err error)
	RoleList(ctx context.Context, req *system.RoleListReq) (res *system.RoleListRes, err error)
	RoleParent(ctx context.Context, req *system.RoleParentReq) (res *system.RoleParentRes, err error)
	RoleMenuList(ctx context.Context, req *system.RoleMenuListReq) (res *system.RoleMenuListRes, err error)
	RoleSave(ctx context.Context, req *system.RoleSaveReq) (res *system.RoleSaveRes, err error)
	RoleStatus(ctx context.Context, req *system.RoleStatusReq) (res *system.RoleStatusRes, err error)
	RoleDel(ctx context.Context, req *system.RoleDelReq) (res *system.RoleDelRes, err error)
	RuleList(ctx context.Context, req *system.RuleListReq) (res *system.RuleListRes, err error)
	RuleParent(ctx context.Context, req *system.RuleParentReq) (res *system.RuleParentRes, err error)
	RuleRoutes(ctx context.Context, req *system.RuleRoutesReq) (res *system.RuleRoutesRes, err error)
	RuleSave(ctx context.Context, req *system.RuleSaveReq) (res *system.RuleSaveRes, err error)
	RuleStatus(ctx context.Context, req *system.RuleStatusReq) (res *system.RuleStatusRes, err error)
	RuleDel(ctx context.Context, req *system.RuleDelReq) (res *system.RuleDelRes, err error)
	EmptyRecyclebin(ctx context.Context, req *system.EmptyRecyclebinReq) (res *system.EmptyRecyclebinRes, err error)
	RuleContent(ctx context.Context, req *system.RuleContentReq) (res *system.RuleContentRes, err error)
}

type IAdminUser interface {
	GetUserinfo(ctx context.Context, req *user.GetUserinfoReq) (res *user.GetUserinfoRes, err error)
	GetMenu(ctx context.Context, req *user.GetMenuReq) (res *user.GetMenuRes, err error)
	SaveInfo(ctx context.Context, req *user.SaveInfoReq) (res *user.SaveInfoRes, err error)
	Login(ctx context.Context, req *user.LoginReq) (res *user.LoginRes, err error)
	LoginOut(ctx context.Context, req *user.LoginOutReq) (res *user.LoginOutRes, err error)
}
