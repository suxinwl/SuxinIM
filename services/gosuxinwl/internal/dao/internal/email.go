// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// EmailDao is the data access object for the table gf_email.
type EmailDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  EmailColumns       // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// EmailColumns defines and stores column names for the table gf_email.
type EmailColumns struct {
	Id          string //
	DataFrom    string // 数据来源sys=后台管理
	BusinessId  string // 业务主账号id
	SenderEmail string // 发送者邮箱
	AuthCode    string // 邮箱授权码
	MailTitle   string // 邮件标题
	MailBody    string // 邮件内容,可以是html
	ServiceHost string // 邮件服务器
	ServicePort string // 邮件服务器端口
}

// emailColumns holds the columns for the table gf_email.
var emailColumns = EmailColumns{
	Id:          "id",
	DataFrom:    "data_from",
	BusinessId:  "business_id",
	SenderEmail: "sender_email",
	AuthCode:    "auth_code",
	MailTitle:   "mail_title",
	MailBody:    "mail_body",
	ServiceHost: "service_host",
	ServicePort: "service_port",
}

// NewEmailDao creates and returns a new DAO object for table data access.
func NewEmailDao(handlers ...gdb.ModelHandler) *EmailDao {
	return &EmailDao{
		group:    "default",
		table:    "gf_email",
		columns:  emailColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *EmailDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *EmailDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *EmailDao) Columns() EmailColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *EmailDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *EmailDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *EmailDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
