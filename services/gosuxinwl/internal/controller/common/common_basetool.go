package common

import (
	"context"
	"time"

	"github.com/suxinwl/GoSuxin/api/common/basetool"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/grand"
)

func (c *ControllerBasetool) GetCaptcha(ctx context.Context, req *basetool.GetCaptchaReq) (res *basetool.GetCaptchaRes, err error) {
	resData := gf.Failed()
	typeStr := req.Type
	switch typeStr {
	case "image": //获取图片验证码
		data, err := gf.GenerateCaptcha(ctx)
		if err != nil {
			resData = gf.Failed().SetMsg("获取图片验证码失败")
		} else {
			resData = gf.Success().SetMsg("获取图片验证码成功").SetData(data)
		}
	case "mobile": //获取手机验证码
		mobile := req.Mobile
		if mobile == "" {
			resData = gf.Failed().SetMsg("请填写手机")
		} else {
			code := grand.Digits(6)
			gf.SetCache(mobile, code, time.Second*60) //验证码存在本地缓存中
			resData = gf.Success().SetMsg("获取手机验证码").SetData(code)
		}
	case "email": //获取邮箱验证码
		email := req.Email
		if email == "" {
			resData = gf.Failed().SetMsg("请填写邮箱")
		} else {
			code := grand.Digits(6)
			res, erro := gf.SendEmail([]string{email}, "", "", code)
			if erro == nil {
				resData = gf.Success().SetMsg("获取验证码").SetData(res)
			} else {
				resData = gf.Failed().SetMsg(erro.Error())
			}
		}
	case "qrcode": //扫码登录
		keyval := req.Keyval
		if keyval == "" {
			resData = gf.Failed().SetMsg("选择扫码类型")
		} else {
			addtime, _ := time.ParseDuration("60s")
			expiretime := time.Now().Add(addtime).UnixMilli()
			resData = gf.Success().SetMsg("获取扫码内容").SetData(g.Map{"url": keyval, "expiretime": expiretime})
		}
	}
	//返回数据
	res = &basetool.GetCaptchaRes{
		R: resData,
	}
	return
}

// 检查菜单路由名是否存在
func (c *ControllerBasetool) CheckHaseRule(ctx context.Context, req *basetool.CheckHaseRuleReq) (res *basetool.CheckHaseRuleRes, err error) {
	resData := gf.Failed()
	data, err := dao.AuthRule.Ctx(ctx).Where("id !=?", req.Id).WhereIn("routename", req.Routename).Value("id")
	if err != nil {
		resData = gf.Failed().SetMsg("检查路由失败").SetData(err)
	} else {
		hase := false
		if data == nil { //可以用
			hase = true
		}
		resData = gf.Success().SetMsg("检查路由成功！").SetData(hase)
	}
	res = &basetool.CheckHaseRuleRes{
		R: resData,
	}
	return
}
