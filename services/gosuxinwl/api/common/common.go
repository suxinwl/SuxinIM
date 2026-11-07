// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package common

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/common/basetool"
)

type ICommonBasetool interface {
	GetDicData(ctx context.Context, req *basetool.GetDicDataReq) (res *basetool.GetDicDataRes, err error)
	Pan123Asset(ctx context.Context, req *basetool.Pan123AssetReq) (res *basetool.Pan123AssetRes, err error)
	Weigh(ctx context.Context, req *basetool.WeighReq) (res *basetool.WeighRes, err error)
	GetTables(ctx context.Context, req *basetool.GetTablesReq) (res *basetool.GetTablesRes, err error)
	GetCaptcha(ctx context.Context, req *basetool.GetCaptchaReq) (res *basetool.GetCaptchaRes, err error)
	CheckHaseRule(ctx context.Context, req *basetool.CheckHaseRuleReq) (res *basetool.CheckHaseRuleRes, err error)
}
