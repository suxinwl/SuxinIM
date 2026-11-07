// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AuthRoleAccessDao is the data access object for the table gf_auth_role_access.
type AuthRoleAccessDao struct {
	table    string                // table is the underlying table name of the DAO.
	group    string                // group is the database configuration group name of the current DAO.
	columns  AuthRoleAccessColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler    // handlers for customized model modification.
}

// AuthRoleAccessColumns defines and stores column names for the table gf_auth_role_access.
type AuthRoleAccessColumns struct {
	Uid    string // 账号id
	RoleId string // 授权id
}

// authRoleAccessColumns holds the columns for the table gf_auth_role_access.
var authRoleAccessColumns = AuthRoleAccessColumns{
	Uid:    "uid",
	RoleId: "role_id",
}

// NewAuthRoleAccessDao creates and returns a new DAO object for table data access.
func NewAuthRoleAccessDao(handlers ...gdb.ModelHandler) *AuthRoleAccessDao {
	return &AuthRoleAccessDao{
		group:    "default",
		table:    "gf_auth_role_access",
		columns:  authRoleAccessColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AuthRoleAccessDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AuthRoleAccessDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AuthRoleAccessDao) Columns() AuthRoleAccessColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AuthRoleAccessDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AuthRoleAccessDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AuthRoleAccessDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
