package user

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/errors/gcode"
	"github.com/suxinwl/GoSuxin/framework/errors/gerror"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/genctrl/multi/api/admin/user/v1"
)

// Create add title.
func (c *ControllerV1) Create(ctx context.Context, req *v1.CreateReq) (res *v1.CreateRes, err error) {
	return nil, gerror.NewCode(gcode.CodeNotImplemented)
}
