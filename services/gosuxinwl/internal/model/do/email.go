// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package do

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// Email is the golang structure of table gf_email for DAO operations like Where/Data.
type Email struct {
	g.Meta      `orm:"table:gf_email, do:true"`
	Id          any //
	DataFrom    any // 数据来源sys=后台管理
	BusinessId  any // 业务主账号id
	SenderEmail any // 发送者邮箱
	AuthCode    any // 邮箱授权码
	MailTitle   any // 邮件标题
	MailBody    any // 邮件内容,可以是html
	ServiceHost any // 邮件服务器
	ServicePort any // 邮件服务器端口
}
