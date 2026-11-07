// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AuthDeptDao is the data access object for the table gf_auth_dept.
type AuthDeptDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AuthDeptColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AuthDeptColumns defines and stores column names for the table gf_auth_dept.
type AuthDeptColumns struct {
	Id             string //
	BusinessId     string // 业务主账号id
	AccountId      string // 添加账号
	Name           string // 部门名称
	Pid            string // 上级部门
	Weigh          string // 排序
	Status         string // 状态:0=正常;1=禁用
	IMGroupEnabled string // 部门群自动管理开关
	LeaderUID      string // 部门负责人宿主UID
	Remark         string // 备注
	Createtime     string // 创建时间
	Updatetime     string // 更新时间
	Deletetime     string // 删除时间
}

// authDeptColumns holds the columns for the table gf_auth_dept.
var authDeptColumns = AuthDeptColumns{
	Id:             "id",
	BusinessId:     "business_id",
	AccountId:      "account_id",
	Name:           "name",
	Pid:            "pid",
	Weigh:          "weigh",
	Status:         "status",
	IMGroupEnabled: "im_group_enabled",
	LeaderUID:      "leader_uid",
	Remark:         "remark",
	Createtime:     "createtime",
	Updatetime:     "updatetime",
	Deletetime:     "deletetime",
}

// NewAuthDeptDao creates and returns a new DAO object for table data access.
func NewAuthDeptDao(handlers ...gdb.ModelHandler) *AuthDeptDao {
	return &AuthDeptDao{
		group:    "default",
		table:    "gf_auth_dept",
		columns:  authDeptColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AuthDeptDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AuthDeptDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AuthDeptDao) Columns() AuthDeptColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AuthDeptDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AuthDeptDao) Ctx(ctx context.Context) *gdb.Model {
	model := dao.DB().Model(dao.table)
	for _, handler := range dao.handlers {
		model = handler(model)
	}
	return model.Safe().Ctx(ctx)
}

// Transaction wraps the transaction logic using function f.
// It rolls back the transaction and returns the error if function f returns a non-nil error.
// It commits the transaction and returns nil if function f returns nil.
//
// Note: Do not commit or roll back the transaction in function f,
// as it is automatically handled by this function.
func (dao *AuthDeptDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
