package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetEmailReq struct {
	g.Meta   `path:"datacenter/configuration/getEmail" tags:"getEmail" method:"get" summary:"获取系统管理邮箱"`
	DataFrom string `p:"data_from" d:"sys" dc:"字典名称"`
}
type GetEmailRes struct {
	*gf.R
}

// 保存数据
type SaveEmailReq struct {
	g.Meta      `path:"datacenter/configuration/saveEmail" tags:"Save" method:"post" summary:"保存邮箱数据"`
	DataFrom    string `p:"data_from" d:"sys" dc:"字典名称"`
	SenderEmail string `p:"sender_email"  d:"" dc:"发送者邮箱"`
	AuthCode    string `p:"auth_code"  d:"" dc:"邮箱授权码"`
	MailTitle   string `p:"mail_title"  d:"" dc:"邮件标题"`
	MailBody    string `p:"mail_body"  d:"" dc:"邮件内容,可以是html"`
	ServiceHost string `p:"service_host"  d:"" dc:"邮件服务器"`
	ServicePort string `p:"service_port"  d:"" dc:"邮件服务器端口"`
}
type SaveEmailRes struct {
	*gf.R
}

/*****************插件配置**********************/
type GetCodestoreConfigReq struct {
	g.Meta `path:"datacenter/configuration/getCodestoreConfig" tags:"GetCodestoreConfig" method:"get" summary:"获取安装的代码仓配置"`
}
type GetCodestoreConfigRes struct {
	*gf.R
}

type SaveCodeStoreConfigReq struct {
	g.Meta `path:"datacenter/configuration/saveCodeStoreConfig" tags:"saveCodeStoreConfig" method:"post" summary:"获修改安装的代码仓配置"`
	Name   string       `p:"name" v:"required#标识名不能为空"  dc:"标识"`
	Status bool         `p:"status" d:"false" dc:"状态"`
	Data   g.ListStrAny `p:"data" d:"[]" dc:"配置数据"`
}
type SaveCodeStoreConfigRes struct {
	*gf.R
}

type UpConfigStatusReq struct {
	g.Meta      `path:"datacenter/configuration/upConfigStatus" tags:"upConfigStatus" method:"post" summary:"修改安装的代码仓配置状态"`
	Name        string `p:"name" v:"required#名称不能为空"  dc:"名称"`
	Pluginident string `p:"pluginident" v:"required#标识名不能为空"  dc:"标识"`
	Status      bool   `p:"status" d:"false" dc:"状态"`
}
type UpConfigStatusRes struct {
	*gf.R
}
