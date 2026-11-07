// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// Attachment is the golang structure for table attachment.
type Attachment struct {
	Id          uint        `json:"id"          orm:"id"          description:"文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档"`
	BusinessId  int         `json:"businessId"  orm:"business_id" description:"业务主账号id"`
	Pid         int         `json:"pid"         orm:"pid"         description:"附件"`
	Location    string      `json:"location"    orm:"location"    description:"图片存储位置:local=本地,alioss=阿里云,tencentcos=腾讯云,qiniuoss=七牛云"`
	Name        string      `json:"name"        orm:"name"        description:"附件原来名称"`
	Title       string      `json:"title"       orm:"title"       description:"文件名称"`
	Type        int         `json:"type"        orm:"type"        description:"文件类型0=图片，1=文件夹,2=视频，3=音频,4=文档,5=其他"`
	Weigh       int         `json:"weigh"       orm:"weigh"       description:"排序"`
	Url         string      `json:"url"         orm:"url"         description:"访问路径"`
	Imagewidth  string      `json:"imagewidth"  orm:"imagewidth"  description:"宽度"`
	Imageheight string      `json:"imageheight" orm:"imageheight" description:"高度"`
	Filesize    uint64      `json:"filesize"    orm:"filesize"    description:"文件大小（字节）"`
	Mimetype    string      `json:"mimetype"    orm:"mimetype"    description:"mime类型"`
	Extparam    string      `json:"extparam"    orm:"extparam"    description:"透传数据"`
	Storage     string      `json:"storage"     orm:"storage"     description:"存储位置"`
	CoverUrl    string      `json:"coverUrl"    orm:"cover_url"   description:"视频封面"`
	Sha1        string      `json:"sha1"        orm:"sha1"        description:"文件 sha1编码"`
	IsCommon    int         `json:"isCommon"    orm:"is_common"   description:"是否公共1=是"`
	Createtime  *gtime.Time `json:"createtime"  orm:"createtime"  description:"上传时间"`
}
