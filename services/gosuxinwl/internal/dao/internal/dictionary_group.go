// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// DictionaryGroupDao is the data access object for the table gf_dictionary_group.
type DictionaryGroupDao struct {
	table    string                 // table is the underlying table name of the DAO.
	group    string                 // group is the database configuration group name of the current DAO.
	columns  DictionaryGroupColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler     // handlers for customized model modification.
}

// DictionaryGroupColumns defines and stores column names for the table gf_dictionary_group.
type DictionaryGroupColumns struct {
	Id         string //
	BusinessId string // 业务主账号id
	Title      string // 字典分组名称
	Remark     string // 备注
	DataFrom   string // 数据来源:common=公共,business=商业端
	DbWay      string // 数据存储位置:sys=公共表,alone=单独建表
	Tablename  string // 数据表名称
	Status     string // 状态
	Weigh      string // 排序
	Createtime string // 创建时间
}

// dictionaryGroupColumns holds the columns for the table gf_dictionary_group.
var dictionaryGroupColumns = DictionaryGroupColumns{
	Id:         "id",
	BusinessId: "business_id",
	Title:      "title",
	Remark:     "remark",
	DataFrom:   "data_from",
	DbWay:      "db_way",
	Tablename:  "tablename",
	Status:     "status",
	Weigh:      "weigh",
	Createtime: "createtime",
}

// NewDictionaryGroupDao creates and returns a new DAO object for table data access.
func NewDictionaryGroupDao(handlers ...gdb.ModelHandler) *DictionaryGroupDao {
	return &DictionaryGroupDao{
		group:    "default",
		table:    "gf_dictionary_group",
		columns:  dictionaryGroupColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *DictionaryGroupDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *DictionaryGroupDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *DictionaryGroupDao) Columns() DictionaryGroupColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *DictionaryGroupDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *DictionaryGroupDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *DictionaryGroupDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
