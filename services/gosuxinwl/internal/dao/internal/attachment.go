// ==========================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// ==========================================================================

package internal

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// AttachmentDao is the data access object for the table gf_attachment.
type AttachmentDao struct {
	table    string             // table is the underlying table name of the DAO.
	group    string             // group is the database configuration group name of the current DAO.
	columns  AttachmentColumns  // columns contains all the column names of Table for convenient usage.
	handlers []gdb.ModelHandler // handlers for customized model modification.
}

// AttachmentColumns defines and stores column names for the table gf_attachment.
type AttachmentColumns struct {
	Id          string // 文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档
	BusinessId  string // 业务主账号id
	Pid         string // 附件
	Location    string // 图片存储位置:local=本地,alioss=阿里云,tencentcos=腾讯云,qiniuoss=七牛云
	Name        string // 附件原来名称
	Title       string // 文件名称
	Type        string // 文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档,5=其他
	Weigh       string // 排序
	Url         string // 访问路径
	Imagewidth  string // 宽度
	Imageheight string // 高度
	Filesize    string // 文件大小（字节）
	Mimetype    string // mime类型
	Extparam    string // 透传数据
	Storage     string // 存储位置
	CoverUrl    string // 视频封面
	Sha1        string // 文件 sha1编码
	IsCommon    string // 是否公共1=是
	Createtime  string // 上传时间
}

// attachmentColumns holds the columns for the table gf_attachment.
var attachmentColumns = AttachmentColumns{
	Id:          "id",
	BusinessId:  "business_id",
	Pid:         "pid",
	Location:    "location",
	Name:        "name",
	Title:       "title",
	Type:        "type",
	Weigh:       "weigh",
	Url:         "url",
	Imagewidth:  "imagewidth",
	Imageheight: "imageheight",
	Filesize:    "filesize",
	Mimetype:    "mimetype",
	Extparam:    "extparam",
	Storage:     "storage",
	CoverUrl:    "cover_url",
	Sha1:        "sha1",
	IsCommon:    "is_common",
	Createtime:  "createtime",
}

// NewAttachmentDao creates and returns a new DAO object for table data access.
func NewAttachmentDao(handlers ...gdb.ModelHandler) *AttachmentDao {
	return &AttachmentDao{
		group:    "default",
		table:    "gf_attachment",
		columns:  attachmentColumns,
		handlers: handlers,
	}
}

// DB retrieves and returns the underlying raw database management object of the current DAO.
func (dao *AttachmentDao) DB() gdb.DB {
	return g.DB(dao.group)
}

// Table returns the table name of the current DAO.
func (dao *AttachmentDao) Table() string {
	return dao.table
}

// Columns returns all column names of the current DAO.
func (dao *AttachmentDao) Columns() AttachmentColumns {
	return dao.columns
}

// Group returns the database configuration group name of the current DAO.
func (dao *AttachmentDao) Group() string {
	return dao.group
}

// Ctx creates and returns a Model for the current DAO. It automatically sets the context for the current operation.
func (dao *AttachmentDao) Ctx(ctx context.Context) *gdb.Model {
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
func (dao *AttachmentDao) Transaction(ctx context.Context, f func(ctx context.Context, tx gdb.TX) error) (err error) {
	return dao.Ctx(ctx).Transaction(ctx, f)
}
