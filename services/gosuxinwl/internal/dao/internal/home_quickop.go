// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// HomeQuickopDao is the data access object for the table gf_home_quickop.
type HomeQuickopDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  HomeQuickopColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// HomeQuickopColumns defines and stores column names for the table gf_home_quickop.
type HomeQuickopColumns struct {
	Id         string //
	BusinessId string // 业务主账号id
	Uid        string // 添加人
	IsCommon   string // 公共1=是
	Type       string // 类型1=外部
	Name       string // 快捷名称
	PathUrl    string // 跳转路径
	Icon       string // 图标
	Weigh      string // 权重
}

// homeQuickopColumns holds the columns for the table gf_home_quickop.
var homeQuickopColumns = HomeQuickopColumns{
	Id:         "id",
	BusinessId: "business_id",
	Uid:        "uid",
	IsCommon:   "is_common",
	Type:       "type",
	Name:       "name",
	PathUrl:    "path_url",
	Icon:       "icon",
	Weigh:      "weigh",
}

// NewHomeQuickopDao creates and returns a new DAO object for table data access.
func NewHomeQuickopDao(handlers ...gdb.ModelHandler) *HomeQuickopDao {
	return &HomeQuickopDao{
		group:    "default",
		table:    "gf_home_quickop",
		columns:  homeQuickopColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *HomeQuickopDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *HomeQuickopDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *HomeQuickopDao) Columns() HomeQuickopColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *HomeQuickopDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *HomeQuickopDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *HomeQuickopDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
