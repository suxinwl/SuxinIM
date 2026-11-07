// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

// AuthRoleAccess is the golang structure for table auth_role_access.
type AuthRoleAccess struct {
	Uid    int `json:"uid"    orm:"uid"     description:"账号id"`
	RoleId int `json:"roleId" orm:"role_id" description:"授权id"`
}
