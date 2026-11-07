// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

// Email is the golang structure for table email.
type Email struct {
	Id          uint   `json:"id"          orm:"id"           description:""`
	DataFrom    string `json:"dataFrom"    orm:"data_from"    description:"数据来源sys=后台管理"`
	BusinessId  int    `json:"businessId"  orm:"business_id"  description:"业务主账号id"`
	SenderEmail string `json:"senderEmail" orm:"sender_email" description:"发送者邮箱"`
	AuthCode    string `json:"authCode"    orm:"auth_code"    description:"邮箱授权码"`
	MailTitle   string `json:"mailTitle"   orm:"mail_title"   description:"邮件标题"`
	MailBody    string `json:"mailBody"    orm:"mail_body"    description:"邮件内容,可以是html"`
	ServiceHost string `json:"serviceHost" orm:"service_host" description:"邮件服务器"`
	ServicePort int    `json:"servicePort" orm:"service_port" description:"邮件服务器端口"`
}
