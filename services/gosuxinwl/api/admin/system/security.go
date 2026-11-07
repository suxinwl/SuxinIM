package system

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type AccountSecurityReq struct {
	g.Meta `path:"/system/account/security" method:"get"`
	ID     int64 `p:"id" v:"required|min:1"`
}
type AccountResetPasswordReq struct {
	g.Meta   `path:"/system/account/resetPassword" method:"post"`
	ID       int64  `json:"id" v:"required|min:1"`
	Password string `json:"password" v:"required|length:32,32#重置密码参数无效|重置密码参数无效"`
}
type AccountUnlockReq struct {
	g.Meta `path:"/system/account/unlock" method:"post"`
	ID     int64 `json:"id" v:"required|min:1"`
}
type LockPolicyReq struct {
	g.Meta `path:"/system/account/lockPolicy" method:"get"`
}
type SaveLockPolicyReq struct {
	g.Meta `path:"/system/account/lockPolicy" method:"post"`
	accountsecurity.LockPolicy
}
type EmployeePolicyReq struct {
	g.Meta `path:"/system/account/accessPolicy" method:"get"`
	ID     int64 `p:"id" v:"required|min:1"`
}
type SaveEmployeePolicyReq struct {
	g.Meta   `path:"/system/account/accessPolicy" method:"post"`
	ID       int64                  `json:"id" v:"required|min:1"`
	Matrix   accountsecurity.Matrix `json:"matrix"`
	Revision int64                  `json:"revision"`
}
type RolePolicyReq struct {
	g.Meta `path:"/system/role/accessPolicy" method:"get"`
	ID     int64 `p:"id" v:"required|min:1"`
}
type SaveRolePolicyReq struct {
	g.Meta   `path:"/system/role/accessPolicy" method:"post"`
	ID       int64                  `json:"id" v:"required|min:1"`
	Matrix   accountsecurity.Matrix `json:"matrix"`
	Revision int64                  `json:"revision"`
}
type SecurityRes struct{ *gf.R }
