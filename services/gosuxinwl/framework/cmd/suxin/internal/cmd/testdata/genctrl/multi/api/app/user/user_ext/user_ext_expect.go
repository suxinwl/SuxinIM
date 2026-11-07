// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package user_ext

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/genctrl/multi/api/app/user/user_ext/v1"
)

type IUserExtV1 interface {
	Create(ctx context.Context, req *v1.CreateReq) (res *v1.CreateRes, err error)
	Update(ctx context.Context, req *v1.UpdateReq) (res *v1.UpdateRes, err error)
}
