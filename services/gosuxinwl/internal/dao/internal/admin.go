// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AdminDao is the data access object for the table gf_admin.
type AdminDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AdminColumns       // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AdminColumns defines and stores column names for the table gf_admin.
type AdminColumns struct {
	Id            string // ID
	AccountId     string // 账号id/记录那个账号添加
	DeptId        string // 部门id
	Username      string // 用户名
	Password      string // 密码
	Salt          string // 密码盐
	Name          string // 姓名
	Nickname      string // 昵称
	Avatar        string // 头像
	Email         string // 电子邮箱
	Mobile        string // 手机号码
	Tel           string // 备用电话用户自己填写
	Status        string // 状态:0=正常,1=禁用
	Remark        string // 备注
	Loginip       string // 登录IP
	Logintime     string // 最后登录时间
	LoginAttempts string // 登录尝试次数
	LockTime      string // 账号锁定时间
	Createtime    string // 创建时间
	Updatetime    string // 更新时间
	Deletetime    string // 删除时间
	PwdResetTime  string // 修改密码时间
}

// adminColumns holds the columns for the table gf_admin.
var adminColumns = AdminColumns{
	Id:            "id",
	AccountId:     "account_id",
	DeptId:        "dept_id",
	Username:      "username",
	Password:      "password",
	Salt:          "salt",
	Name:          "name",
	Nickname:      "nickname",
	Avatar:        "avatar",
	Email:         "email",
	Mobile:        "mobile",
	Tel:           "tel",
	Status:        "status",
	Remark:        "remark",
	Loginip:       "loginip",
	Logintime:     "logintime",
	LoginAttempts: "login_attempts",
	LockTime:      "lock_time",
	Createtime:    "createtime",
	Updatetime:    "updatetime",
	Deletetime:    "deletetime",
	PwdResetTime:  "pwd_reset_time",
}

// NewAdminDao creates and returns a new DAO object for table data access.
func NewAdminDao(handlers ...gdb.ModelHandler) *AdminDao {
	return &AdminDao{
		group:    "default",
		table:    "gf_admin",
		columns:  adminColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AdminDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AdminDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AdminDao) Columns() AdminColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AdminDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AdminDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AdminDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
