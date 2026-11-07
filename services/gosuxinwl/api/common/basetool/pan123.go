package basetool

import "github.com/suxinwl/GoSuxin/framework/frame/g"

type Pan123AssetReq struct {
	g.Meta `path:"/storage/pan123/{uid}/{fileID}/{name}" method:"get,head" noValApi:"1" summary:"公开123云盘附件"`
	UID    int64  `p:"uid"`
	FileID int64  `p:"fileID"`
	Name   string `p:"name"`
}
type Pan123AssetRes struct{}
