// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Attachment is the golang structure of table gf_attachment for DAO operations like Where/Data.
type Attachment struct {
	g.Meta      `orm:"table:gf_attachment, do:true"`
	Id          any         // 文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档
	BusinessId  any         // 业务主账号id
	Pid         any         // 附件
	Location    any         // 图片存储位置:local=本地,alioss=阿里云,tencentcos=腾讯云,qiniuoss=七牛云
	Name        any         // 附件原来名称
	Title       any         // 文件名称
	Type        any         // 文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档,5=其他
	Weigh       any         // 排序
	Url         any         // 访问路径
	Imagewidth  any         // 宽度
	Imageheight any         // 高度
	Filesize    any         // 文件大小（字节）
	Mimetype    any         // mime类型
	Extparam    any         // 透传数据
	Storage     any         // 存储位置
	CoverUrl    any         // 视频封面
	Sha1        any         // 文件 sha1编码
	IsCommon    any         // 是否公共1=是
	Createtime  *gtime.Time // 上传时间
}
