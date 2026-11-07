package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

type UploadReq struct {
	g.Meta   `path:"datacenter/upfile/upload" tags:"upload" method:"post" summary:"管理后台上传附件"`
	File     *ghttp.UploadFile `p:"file" type:"file" v:"required" dc:"前端传过来的文件流"`
	Pid      int64             `p:"pid" d:"0" dc:"父级id"`
	Filetype string            `p:"filetype" d:"image" dc:"文件类型"`
}
type UploadRes struct {
	*gf.R
}
