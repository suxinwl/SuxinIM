// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

// HomeQuickop is the golang structure for table home_quickop.
type HomeQuickop struct {
	Id         uint   `json:"id"         orm:"id"          description:""`
	BusinessId int    `json:"businessId" orm:"business_id" description:"业务主账号id"`
	Uid        int    `json:"uid"        orm:"uid"         description:"添加人"`
	IsCommon   int    `json:"isCommon"   orm:"is_common"   description:"公共1=是"`
	Type       int    `json:"type"       orm:"type"        description:"类型1=外部"`
	Name       string `json:"name"       orm:"name"        description:"快捷名称"`
	PathUrl    string `json:"pathUrl"    orm:"path_url"    description:"跳转路径"`
	Icon       string `json:"icon"       orm:"icon"        description:"图标"`
	Weigh      int    `json:"weigh"      orm:"weigh"       description:"权重"`
}
