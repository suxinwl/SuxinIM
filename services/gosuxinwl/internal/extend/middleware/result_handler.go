package middleware

import (
	"github.com/suxinwl/GoSuxin/utility/gf"
	"net/http"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

func HandlerResponse(r *ghttp.Request) {

	r.Middleware.Next()

	// There's custom buffer content, it then exits current handler.
	if r.Response.BufferLength() > 0 || r.Response.BytesWritten() > 0 || r.Response.IsHeaderWrote() {
		return
	}

	var (
		msg string
		err = r.GetError()
		res = r.GetHandlerResponse()
	)

	if err != nil {
		msg = err.Error()
	} else {
		if r.Response.Status > 0 && r.Response.Status != http.StatusOK {
			msg = http.StatusText(r.Response.Status)
		}
	}

	// 返回接口数据
	if nil != res {
		r.Response.WriteJsonExit(res)
	} else {
		r.Response.WriteJsonExit(gf.Failed().SetMsg(msg))
	}

}
