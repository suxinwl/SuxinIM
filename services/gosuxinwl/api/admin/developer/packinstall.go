// ==================
// 打包、安装、上传
// 代码仓
// ==================
package developer

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// 打包
type PackCodeReq struct {
	g.Meta       `path:"developer/packinstall/packCode" tags:"getCodeCate" method:"post" summary:"打包"`
	Title        string           `p:"title" v:"required#包名不能为空" dc:"代码包名"`
	Name         string           `p:"name" v:"required#名称不能为空" dc:"代码名称"`
	Version      string           `p:"version" v:"required#版本号不能为空" dc:"版本号"`
	Packtables   string           `p:"packtables" d:"" dc:"数据表"`
	Des          string           `p:"des" d:"" dc:"描述"`
	Menujson     []any            `p:"menujson" d:"[]" dc:"后台菜单json数据"`
	Menutree     []int64          `p:"menutree" d:"[]" dc:"后台菜单"`
	Installcover bool             `p:"installcover" d:"false" dc:"安装方式"`
	IsModTidy    bool             `p:"isModTidy" d:"false" dc:"更新依赖"`
	CommandLines string           `p:"commandLines" d:"" dc:"Vue前端依赖命令行"`
	GoFiles      []map[string]any `p:"goFiles" d:"[]" dc:"后端代码"`
	VueFiles     []map[string]any `p:"vueFiles" d:"[]" dc:"前端代码"`
}
type PackCodeRes struct {
	*gf.R
}

// 上传代码到代码仓
type UpfileReq struct {
	g.Meta    `path:"developer/packinstall/upfile" tags:"upfile" method:"post" summary:"上传代码到代码仓"`
	File      *ghttp.UploadFile `p:"file" type:"file" v:"required" dc:"前端传过来的文件流"`
	Cid       int64             `p:"cid" d:"0" dc:"分类"`
	Domainurl string            `p:"domainurl" v:"required#代码仓路径不能为空" dc:"代码仓路径"`
	CodeToken string            `p:"code_token" d:"" dc:"请求权限"`
}
type UpfileRes struct {
	*gf.R
}

/******************安装*****************/
// 下载插件代码到本地
type DownCodeReq struct {
	g.Meta    `path:"developer/packinstall/downCode" tags:"downCode" method:"post" summary:"下载插件代码"`
	Baseurl   string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	CodeToken string `p:"code_token" dc:"用户身份标识"`
	Id        int64  `p:"id" v:"required#插件ID不能为空" dc:"插件ID"`
	Name      string `p:"name" v:"required#安装包名称不能为空" dc:"插件名称"`
	Frame     string `p:"frame" d:"goframe" dc:"请求框架"`
}
type DownCodeRes struct {
	*gf.R
}

// 安装插件
type InstallCodeReq struct {
	g.Meta `path:"developer/packinstall/installCode" tags:"installCode" method:"post" summary:"安装插件"`
	Name   string `p:"name" v:"required|regex:^[A-Za-z0-9][A-Za-z0-9_-]*$#插件包名称不能为空#插件包名称格式无效" dc:"插件名称"`
}
type InstallCodeRes struct {
	*gf.R
}

// 卸载插件
type UninstallCodeReq struct {
	g.Meta `path:"developer/packinstall/uninstallCode" tags:"uninstallCode" method:"post" summary:"卸载插件"`
	Name   string `p:"name" v:"required|regex:^[A-Za-z0-9][A-Za-z0-9_-]*$#插件包名称不能为空#插件包名称格式无效" dc:"插件名称"`
}
type UninstallCodeRes struct {
	*gf.R
}

// 安装本地插件
type InstallLocalCodeReq struct {
	g.Meta `path:"developer/packinstall/installLocalCode" tags:"installLocalCode" method:"post" summary:"安装本地插件"`
	File   *ghttp.UploadFile `p:"file" type:"file" v:"required" dc:"前端传过来的文件流"`
}
type InstallLocalCodeRes struct {
	*gf.R
}

// 查找本地已经安装的包
type GetInstallPackReq struct {
	g.Meta  `path:"developer/packinstall/getInstallPack" tags:"getInstallPack" method:"get" summary:"查找本地已经安装的包"`
	Catalog bool `p:"catalog" dc:"返回本地插件目录及安装状态"`
}
type GetInstallPackRes struct {
	*gf.R
}
