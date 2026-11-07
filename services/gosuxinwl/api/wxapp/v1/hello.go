package v1

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 忽略接口加密验证（noValApi:"1"）
type HelloReq struct {
	g.Meta `path:"/hello" tags:"Hello" noValApi:"1" method:"get" summary:"You first hello api"`
	Token  string `p:"token" d:"" dc:"加密密文"`
}
type HelloRes struct {
	*gf.R
}
