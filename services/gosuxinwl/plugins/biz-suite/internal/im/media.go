package im

import (
	"github.com/suxinwl/GoSuxin/api/admin/datacenter"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	_ "github.com/suxinwl/GoSuxin/internal/logic/admin_datacenter"
	"github.com/suxinwl/GoSuxin/internal/service"
)

// Delegate to the host's upload service so allowlisted extensions, size limits,
// storage providers, deduplication and attachment rows stay consistent.
func uploadMedia(r *ghttp.Request) {
	file := r.GetUploadFile("file")
	if file == nil {
		reply(r, nil, ErrInvalid)
		return
	}
	kind := r.Get("filetype").String()
	if kind != "image" && kind != "file" && kind != "audio" && kind != "video" {
		kind = "file"
	}
	r.Response.WriteJson(service.Admindatacenter().Upload(r.Context(), &datacenter.UploadReq{File: file, Filetype: kind, Pid: 0}))
}
