package user_ext

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/errors/gcode"
	"github.com/suxinwl/GoSuxin/framework/errors/gerror"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/genctrl/multi/api/app/user/user_ext/v1"
)

func (c *ControllerV1) Update(ctx context.Context, req *v1.UpdateReq) (res *v1.UpdateRes, err error) {
	return nil, gerror.NewCode(gcode.CodeNotImplemented)
}
