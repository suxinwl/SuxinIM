package user

import (
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

type EffectivePolicyReq struct {
	g.Meta `path:"/user/effectivePolicy" method:"get" noAuth:"1"`
}
type EffectivePolicyRes struct{ *gf.R }
