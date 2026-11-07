// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// LoginLogDao is the data access object for the table gf_login_log.
type LoginLogDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  LoginLogColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// LoginLogColumns defines and stores column names for the table gf_login_log.
type LoginLogColumns struct {
	Id         string //
	Uid        string // 用户id
	Username   string // 登录账号
	Ip         string // 登录IP
	Address    string // 地点
	Des        string // 登录行为
	Os         string // 操作系统
	Browser    string // 浏览器类型
	ErrorMsg   string // 登录失败原因
	Status     string // 状态:0=成功,1=失败
	Createtime string // 创建时间
}

// loginLogColumns holds the columns for the table gf_login_log.
var loginLogColumns = LoginLogColumns{
	Id:         "id",
	Uid:        "uid",
	Username:   "username",
	Ip:         "ip",
	Address:    "address",
	Des:        "des",
	Os:         "os",
	Browser:    "browser",
	ErrorMsg:   "error_msg",
	Status:     "status",
	Createtime: "createtime",
}

// NewLoginLogDao creates and returns a new DAO object for table data access.
func NewLoginLogDao(handlers ...gdb.ModelHandler) *LoginLogDao {
	return &LoginLogDao{
		group:    "default",
		table:    "gf_login_log",
		columns:  loginLogColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *LoginLogDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *LoginLogDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *LoginLogDao) Columns() LoginLogColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *LoginLogDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *LoginLogDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *LoginLogDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
