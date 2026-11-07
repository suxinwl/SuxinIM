// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package wxapp

import (
	"context"

	"github.com/suxinwl/GoSuxin/api/wxapp/v1"
)

type IWxappV1 interface {
	Hello(ctx context.Context, req *v1.HelloReq) (res *v1.HelloRes, err error)
}
