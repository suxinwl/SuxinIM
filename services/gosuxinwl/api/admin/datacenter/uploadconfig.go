package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetUploadconfigReq struct {
	g.Meta `path:"datacenter/uploadconfig/getConfig" tags:"getConfig" method:"get" summary:"获取文件上传配置"`
}
type GetUploadconfigRes struct {
	*gf.R
}

// 保存应用配置数据
type SaveUploadconfigReq struct {
	g.Meta         `path:"datacenter/uploadconfig/saveConfig" tags:"saveConfig" method:"post" summary:"保存文件上传配置数据"`
	Type           string `p:"Type" d:"local" dc:"文件存储方式"`
	MaxBodySize    int64  `p:"MaxBodySize" d:"600" dc:"传输文件最大值"`
	AllowedExt     string `p:"AllowedExt" d:".jpg,.jpeg,.png" dc:"可上传文件类型"`
	BaseUrl        string `p:"BaseUrl" dc:"文件访问路径"`
	DirPath        string `p:"DirPath" d:"/resource/uploads/" dc:"上传附件路径"`
	Endpoint       string `p:"Endpoint" dc:"自定义访问域名"`
	KeyId          string `p:"KeyId" dc:"密钥Key或秘钥id"`
	Secret         string `p:"Secret" dc:"密钥SecretKey"`
	BucketName     string `p:"BucketName" dc:"空间名称"`
	DestBucketName string `p:"DestBucketName" dc:"移动目标空间名称"`
	Region         string `p:"Region" dc:"所属地域"`
	UseHTTPS       bool   `p:"UseHTTPS" d:"false" dc:"是否使用https"`
	Zone           int    `p:"Zone" dc:"存储区域"`
	Pan123Fields
}
type SaveUploadconfigRes struct {
	*gf.R
}

type Pan123Fields struct {
	ClientID     string `p:"clientID"`
	ClientSecret string `p:"clientSecret"`
	ParentID     int64  `p:"parentId" v:"min:0"`
	URLAuth      *bool  `p:"urlAuth"`
	CDNKey       string `p:"cdnKey"`
}
type TestConnectionReq struct {
	g.Meta `path:"datacenter/uploadconfig/testConnection" tags:"testConnection" method:"post" summary:"测试123云盘连接与上传"`
	Pan123Fields
}
type TestConnectionRes struct{ *gf.R }
