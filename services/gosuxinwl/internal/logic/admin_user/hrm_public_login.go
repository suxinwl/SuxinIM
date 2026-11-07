package adminuser

import (
	"context"
	"errors"
	"strconv"

	"github.com/suxinwl/GoSuxin/internal/accountsecurity"
	"github.com/suxinwl/GoSuxin/internal/imbridge"
	"github.com/suxinwl/GoSuxin/internal/model/entity"
	"github.com/suxinwl/GoSuxin/utility/auth"
)

func generateHRMLoginToken(ctx context.Context, account entity.Admin) (string, error) {
	if err := imbridge.CheckHRMBackofficeLogin(ctx, int64(account.Id)); err != nil {
		return "", err
	}
	claims, err := imbridge.HRMAssetLoginClaims(ctx, int64(account.Id), account.Username, account.Password, account.Salt)
	if err != nil {
		return "", err
	}
	security, err := imbridge.HostSecurityClaims(ctx, int64(account.Id))
	if err != nil {
		return "", err
	}
	if account.AuthenticatedEpoch > 0 && security["securityVersion"] != account.AuthenticatedEpoch {
		return "", errors.New("account authentication changed")
	}
	if security["credentialFingerprint"] != accountsecurity.Hash(account.Password+":"+account.Salt) {
		return "", errors.New("account authentication changed")
	}
	for key, value := range security {
		claims[key] = value
	}
	opaque, err := auth.GenerateToken(ctx, strconv.FormatInt(int64(account.Id), 10), claims)
	if err == nil {
		imbridge.SetHRMAssetLoginCookie(ctx, opaque)
	}
	return opaque, err
}

func hrmLoginTokenError(err error) string {
	if errors.Is(err, imbridge.ErrHRMPublicAdminRequired) {
		return imbridge.ErrHRMPublicAdminRequired.Error()
	}
	return "登录失败，请稍后再试"
}
