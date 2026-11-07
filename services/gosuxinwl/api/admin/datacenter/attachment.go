package datacenter

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 获取我的附件
type GetMyFilesReq struct {
	g.Meta     `path:"datacenter/attachment/getMyFiles" tags:"getMyFiles" method:"get" summary:"获取我的附件"`
	Searchword string `p:"searchword" d:"" dc:"搜索关键词"`
	Pid        int64  `p:"pid" d:"0" dc:"父级id"`
	Filetype   string `p:"filetype" d:"filetype" dc:"文件类型"`
}
type GetMyFilesRes struct {
	*gf.R
}

// 新建文件夹
type SaveReq struct {
	g.Meta `path:"datacenter/attachment/save" tags:"save" method:"post" summary:"创建文件夹"`
	Id     int64  `p:"id" d:"0" dc:"id"`
	Pid    int64  `p:"pid" d:"0" dc:"父级id"`
	Title  string `p:"title" dc:"新建文件夹名称"`
	Type   int64  `p:"type"  dc:"文件类型"`
}
type SaveRes struct {
	*gf.R
}

// 删除文件夹
type DelDirReq struct {
	g.Meta `path:"datacenter/attachment/delDir" tags:"delDir" method:"delete" summary:"删除文件夹"`
	Id     int64 `p:"id" v:"required#删除的id不能为空" dc:"id"`
}
type DelDirRes struct {
	*gf.R
}

// 删除文件
type DelReq struct {
	g.Meta `path:"datacenter/attachment/del" tags:"del" method:"delete" summary:"删除文件"`
	Ids    []int64 `p:"ids" v:"required#删除文件的ids不能为空" dc:"删除文件的id"`
}
type DelRes struct {
	*gf.R
}

// 移动文件到文件夹
type UpImgPidReq struct {
	g.Meta `path:"datacenter/attachment/upImgPid" tags:"upImgPid" method:"post" summary:"删除文件夹"`
	Imgid  int64 `p:"imgid" v:"required#文件id不能为空" dc:"文件id"`
	Pid    int64 `p:"pid" v:"required#文件夹ID不能为空" dc:"文件夹ID"`
}
type UpImgPidRes struct {
	*gf.R
}
