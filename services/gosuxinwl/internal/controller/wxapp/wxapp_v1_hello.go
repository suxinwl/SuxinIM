package wxapp

import (
	"context"
	"net/http"

	v1 "github.com/suxinwl/GoSuxin/api/wxapp/v1"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

func (c *ControllerV1) Hello(ctx context.Context, req *v1.HelloReq) (res *v1.HelloRes, err error) {
	// The scaffold endpoint must never mint a host administrator credential.
	if r := ghttp.RequestFromCtx(ctx); r != nil {
		r.Response.WriteStatus(http.StatusNotFound)
	}
	return nil, nil
}
