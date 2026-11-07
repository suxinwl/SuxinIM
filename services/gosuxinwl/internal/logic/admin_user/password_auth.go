package adminuser

import (
	"context"
	"errors"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gcfg"
	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/dao"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

// AuthenticatePassword is shared by the original HRM login and IM SSO. The
// input password is the existing browser-side MD5 value, not plaintext.
func (s *sAdminUser) AuthenticatePassword(ctx context.Context, username, password, captchaID, captcha string) (*entity.Admin, error) {
	fail := errors.New("账号、密码或验证码无效")
	if strings.TrimSpace(username) == "" || password == "" {
		return nil, fail
	}
	var data entity.Admin
	if err := dao.Admin.Ctx(ctx).Where("username", username).Scan(&data); err != nil || data.Id == 0 {
		return nil, fail
	}
	if data.Status == gf.StatusNo || data.Deletetime != nil {
		return nil, fail
	}
	enabled, _ := gcfg.Instance("app").Get(ctx, "app.loginCaptcha")
	if enabled.Bool() && (captchaID == "" || captcha == "" || !gf.VerifyCaptcha(captchaID, captcha)) {
		clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": username, "status": 1, "des": "账号登录", "error_msg": "输入的验证码不正确"})
		return nil, fail
	}
	store, err := accountsecurity.Default(ctx)
	if err != nil {
		return nil, errors.New("账号安全服务暂不可用")
	}
	if data.AuthenticatedEpoch, err = store.AuthenticateVersion(ctx, int64(data.Id), password); err != nil {
		clogic.AddloginLog(ctx, g.Map{"uid": data.Id, "username": username, "status": 1, "des": "账号登录", "error_msg": "账号验证未通过"})
		return nil, fail
	}
	return &data, nil
}
