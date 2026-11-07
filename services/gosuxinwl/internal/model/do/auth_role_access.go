// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AuthRoleAccess is the golang structure of table gf_auth_role_access for DAO operations like Where/Data.
type AuthRoleAccess struct {
	g.Meta `orm:"table:gf_auth_role_access, do:true"`
	Uid    any // 账号id
	RoleId any // 授权id
}
