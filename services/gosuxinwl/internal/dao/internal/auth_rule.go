// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AuthRuleDao is the data access object for the table gf_auth_rule.
type AuthRuleDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AuthRuleColumns    // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AuthRuleColumns defines and stores column names for the table gf_auth_rule.
type AuthRuleColumns struct {
	Id                 string //
	Uid                string // 添加用户
	Title              string // 菜单名称
	Des                string // 描述
	Locale             string // 中英文标题key
	Weigh              string // 排序
	Type               string // 类型 0=目录，1=菜单，2=按钮
	Pid                string // 上一级
	Icon               string // 图标
	Routepath          string // 路由地址
	Routename          string // 路由名称
	Component          string // 组件路径
	Redirect           string // 重定向地址
	Path               string // 接口路径
	Permission         string // 权限标识
	Status             string // 状态 0=启用1=禁用
	Isext              string // 是否外链 0=否1=是
	Keepalive          string // 是否缓存 0=否1=是
	Requiresauth       string // 是否需要登录鉴权 0=否1=是
	Hideinmenu         string // 是否在左侧菜单中隐藏该项 0=否1=是
	Hidechildreninmenu string // 强制在左侧菜单中显示单项 0=否1=是
	Activemenu         string // 高亮设置的菜单项 0=否1=是
	Noaffix            string // 如果设置为true，标签将不会添加到tab-bar中 0=否1=是
	Onlypage           string // 独立页面不需layout和登录，如登录页、数据大屏
	Createtime         string // 创建时间
	Updatetime         string // 更新时间
	Deletetime         string // 删除时间
}

// authRuleColumns holds the columns for the table gf_auth_rule.
var authRuleColumns = AuthRuleColumns{
	Id:                 "id",
	Uid:                "uid",
	Title:              "title",
	Des:                "des",
	Locale:             "locale",
	Weigh:              "weigh",
	Type:               "type",
	Pid:                "pid",
	Icon:               "icon",
	Routepath:          "routepath",
	Routename:          "routename",
	Component:          "component",
	Redirect:           "redirect",
	Path:               "path",
	Permission:         "permission",
	Status:             "status",
	Isext:              "isext",
	Keepalive:          "keepalive",
	Requiresauth:       "requiresauth",
	Hideinmenu:         "hideinmenu",
	Hidechildreninmenu: "hidechildreninmenu",
	Activemenu:         "activemenu",
	Noaffix:            "noaffix",
	Onlypage:           "onlypage",
	Createtime:         "createtime",
	Updatetime:         "updatetime",
	Deletetime:         "deletetime",
}

// NewAuthRuleDao creates and returns a new DAO object for table data access.
func NewAuthRuleDao(handlers ...gdb.ModelHandler) *AuthRuleDao {
	return &AuthRuleDao{
		group:    "default",
		table:    "gf_auth_rule",
		columns:  authRuleColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AuthRuleDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AuthRuleDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AuthRuleDao) Columns() AuthRuleColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AuthRuleDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AuthRuleDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AuthRuleDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
