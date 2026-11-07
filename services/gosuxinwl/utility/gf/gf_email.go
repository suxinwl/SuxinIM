package gf

import (
	"errors"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"gopkg.in/gomail.v2"
)

// 发送邮件
// 请求参数：email邮箱地址，title邮件标题(如果为空则默认则从配置获取)，text邮件内容(如果为空则默认则从配置获取)
// 返回参数：bool 结果, error 错误提示
func SendEmail(email []string, title, text, code string) (bool, error) {
	if len(email) == 0 {
		return false, errors.New("请填写邮箱")
	} else {
		emailConfig, _ := g.Model("email").Where("data_from", "sys").One()
		if emailConfig == nil {
			return false, errors.New("请到业务端后台“配置管理”配置邮箱")
		} else {
			sender := emailConfig["sender_email"].String()  //发送者邮箱
			authCode := emailConfig["auth_code"].String()   //邮箱授权码
			mailTitle := emailConfig["mail_title"].String() //邮件标题
			mailBody := emailConfig["mail_body"].String()   //邮件内容,可以是html
			if title != "" {
				mailTitle = title //邮件标题
			}
			if text == "" {
				mailBody = strings.Replace(mailBody, "{code}", code, 1)
				for _, val := range email {
					SetVerifyCode(val, code) //验证码存在本地缓存
				}
			} else {

			}
			m := gomail.NewMessage()
			m.SetHeader("From", sender)       //发送者邮箱账号
			m.SetHeader("To", email...)       //接收者邮箱列表
			m.SetHeader("Subject", mailTitle) //邮件标题
			m.SetBody("text/html", mailBody)  //邮件内容,可以是html
			//服务器地址和端口是默认腾讯的
			service_host := "smtp.qq.com"
			if _, ok := emailConfig["service_host"]; ok {
				service_host = gconv.String(emailConfig["service_host"])
			}
			service_port := 587
			if _, ok := emailConfig["service_port"]; ok {
				service_port = gconv.Int(emailConfig["service_port"])
			}
			d := gomail.NewDialer(service_host, service_port, sender, authCode)
			err := d.DialAndSend(m)
			if err != nil {
				return false, err
			} else {
				return true, nil
			}
		}
	}
}
