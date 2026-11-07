// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// DictionaryDataDao is the data access object for the table gf_dictionary_data.
type DictionaryDataDao struct {
	table    string                // table is the underlying table name of the DAO.
	group    string                // group is the database configuration group name of the current DAO.
	columns  DictionaryDataColumns // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler    // handlers for customized model modification.
}

// DictionaryDataColumns defines and stores column names for the table gf_dictionary_data.
type DictionaryDataColumns struct {
	Id         string //
	DataFrom   string // 数据来源:common=公共,business=商业端
	GroupId    string // 数据分组id
	Keyname    string // 字典名称
	Keyvalue   string // 字典项值
	Tagcolor   string // 标签颜色
	Des        string // 字典描述
	Status     string // 状态
	Weigh      string // 排序
	Createtime string // 创建时间
	Updatetime string // 更新时间
}

// dictionaryDataColumns holds the columns for the table gf_dictionary_data.
var dictionaryDataColumns = DictionaryDataColumns{
	Id:         "id",
	DataFrom:   "data_from",
	GroupId:    "group_id",
	Keyname:    "keyname",
	Keyvalue:   "keyvalue",
	Tagcolor:   "tagcolor",
	Des:        "des",
	Status:     "status",
	Weigh:      "weigh",
	Createtime: "createtime",
	Updatetime: "updatetime",
}

// NewDictionaryDataDao creates and returns a new DAO object for table data access.
func NewDictionaryDataDao(handlers ...gdb.ModelHandler) *DictionaryDataDao {
	return &DictionaryDataDao{
		group:    "default",
		table:    "gf_dictionary_data",
		columns:  dictionaryDataColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *DictionaryDataDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *DictionaryDataDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *DictionaryDataDao) Columns() DictionaryDataColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *DictionaryDataDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *DictionaryDataDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *DictionaryDataDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
