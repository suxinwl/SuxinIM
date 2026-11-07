package article

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/errors/gcode"
	"github.com/suxinwl/GoSuxin/framework/errors/gerror"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/genctrl/default/api/article/v2"
)

func (c *ControllerV2) Update(ctx context.Context, req *v2.UpdateReq) (res *v2.UpdateRes, err error) {
	return nil, gerror.NewCode(gcode.CodeNotImplemented)
}
