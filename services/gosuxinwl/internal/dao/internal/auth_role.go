// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AuthRoleDao is the data access object for the table gf_auth_role.
type AuthRoleDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AuthRoleColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AuthRoleColumns defines and stores column names for the table gf_auth_role.
type AuthRoleColumns struct {
	Id         string //
	BusinessId string // 业务主账号id
	AccountId  string // 添加用户id
	Pid        string // 父级
	Name       string // 名称
	Rules      string // 规则ID 所拥有的权限包括父级
	Menu       string // 选择的id，用于编辑赋值
	Btns       string // 按钮id，用于编辑赋值
	Status     string // 状态1=禁用
	DataAccess string // 数据权限0=自己1=自己及子权限，2=全部
	Remark     string // 描述
	Weigh      string // 排序
	Createtime string // 添加时间
}

// authRoleColumns holds the columns for the table gf_auth_role.
var authRoleColumns = AuthRoleColumns{
	Id:         "id",
	BusinessId: "business_id",
	AccountId:  "account_id",
	Pid:        "pid",
	Name:       "name",
	Rules:      "rules",
	Menu:       "menu",
	Btns:       "btns",
	Status:     "status",
	DataAccess: "data_access",
	Remark:     "remark",
	Weigh:      "weigh",
	Createtime: "createtime",
}

// NewAuthRoleDao creates and returns a new DAO object for table data access.
func NewAuthRoleDao(handlers ...gdb.ModelHandler) *AuthRoleDao {
	return &AuthRoleDao{
		group:    "default",
		table:    "gf_auth_role",
		columns:  authRoleColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AuthRoleDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AuthRoleDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AuthRoleDao) Columns() AuthRoleColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AuthRoleDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AuthRoleDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AuthRoleDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
