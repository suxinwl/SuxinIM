// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// MemberDao is the data access object for the table gf_member.
type MemberDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  MemberColumns      // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// MemberColumns defines and stores column names for the table gf_member.
type MemberColumns struct {
	Id             string // ID
	BusinessId     string // 业务主账号id
	Username       string // 用户名
	Name           string // 姓名
	Nickname       string // 昵称
	Remark         string // 备注
	Password       string // 密码
	Salt           string // 密码盐
	Email          string // 电子邮箱
	Mobile         string // 手机号
	Avatar         string // 头像
	Level          string // 等级
	Sex            string // 性别:1=男性,2=女性,0=未知
	Birthday       string // 出生日期
	Money          string // 余额
	Score          string // 积分
	Successions    string // 连续登录天数
	Maxsuccessions string // 最大连续登录天数
	Prevtime       string // 上次登录时间
	Logintime      string // 登录时间
	Loginip        string // 登录IP
	Loginfailure   string // 失败次数
	Status         string // 状态
	Createtime     string // 创建时间
	Updatetime     string // 更新时间
	Deletetime     string // 删除时间
}

// memberColumns holds the columns for the table gf_member.
var memberColumns = MemberColumns{
	Id:             "id",
	BusinessId:     "business_id",
	Username:       "username",
	Name:           "name",
	Nickname:       "nickname",
	Remark:         "remark",
	Password:       "password",
	Salt:           "salt",
	Email:          "email",
	Mobile:         "mobile",
	Avatar:         "avatar",
	Level:          "level",
	Sex:            "sex",
	Birthday:       "birthday",
	Money:          "money",
	Score:          "score",
	Successions:    "successions",
	Maxsuccessions: "maxsuccessions",
	Prevtime:       "prevtime",
	Logintime:      "logintime",
	Loginip:        "loginip",
	Loginfailure:   "loginfailure",
	Status:         "status",
	Createtime:     "createtime",
	Updatetime:     "updatetime",
	Deletetime:     "deletetime",
}

// NewMemberDao creates and returns a new DAO object for table data access.
func NewMemberDao(handlers ...gdb.ModelHandler) *MemberDao {
	return &MemberDao{
		group:    "default",
		table:    "gf_member",
		columns:  memberColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *MemberDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *MemberDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *MemberDao) Columns() MemberColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *MemberDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *MemberDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *MemberDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
