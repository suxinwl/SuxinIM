// ==================
// 开发者工具
// 代码仓
// ==================
package developer

import (
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

type GetCodeCateReq struct {
	g.Meta  `path:"developer/codestore/getCodeCate" tags:"getCodeCate" method:"get" summary:"获取公共仓分类"`
	Baseurl string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
}
type GetCodeCateRes struct {
	*gf.R
}

type CodeListReq struct {
	g.Meta     `path:"developer/codestore/codeList" tags:"codeList" method:"post" summary:"获取代码仓数据"`
	Baseurl    string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Cid        any    `p:"cid" v:"required#分类不能为空" dc:"分类"`
	CodeToken  any    `p:"code_token" d:"" dc:"请求权限"`
	Innames    any    `p:"innames" d:""`
	Page       any    `p:"page" d:"1"`
	PageSize   any    `p:"pageSize" d:"10"`
	Searchword any    `p:"searchword" d:""`
	Type       any    `p:"type" d:""`
	Frame      any    `p:"frame" d:"goframe"`
}
type CodeListRes struct {
	*gf.R
}

// 登录社区账号
type LoginReq struct {
	g.Meta   `path:"developer/codestore/login" tags:"login" method:"post" summary:"登录社区账号"`
	Baseurl  string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Username any    `p:"username" v:"required#账号不能为空" dc:"账号"`
	Password any    `p:"password" v:"required#密码不能为空" dc:"密码"`
}
type LoginRes struct {
	*gf.R
}

// 检查版本更新
type AsyncVersionReq struct {
	g.Meta    `path:"developer/codestore/asyncVersion" tags:"asyncVersion" method:"post" summary:"检查版本更新"`
	Baseurl   string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	CodeToken any    `p:"code_token" dc:"登录授权"`
}
type AsyncVersionRes struct {
	*gf.R
}

// 更新私有仓地址
type UpPrivateHouseReq struct {
	g.Meta       `path:"developer/codestore/upPrivateHouse" tags:"upPrivateHouse" method:"post" summary:"更新私有仓地址"`
	PrivateHouse string `p:"privateHouse" v:"required#私有仓地址不能为空" dc:"私有仓地址"`
}
type UpPrivateHouseRes struct {
	*gf.R
}

// 检查插件标识是否可用
type CheckPackNameReq struct {
	g.Meta  `path:"developer/codestore/checkPackName" tags:"checkPackName" method:"post" summary:"检查插件标识是否可用"`
	Baseurl string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Name    string `p:"name" v:"required#标识不能为空" dc:"标识"`
	Type    string `p:"type" v:"required#类型不能为空" dc:"类型"`
}
type CheckPackNameRes struct {
	*gf.R
}

// 添加插件标识
type SavePackNameReq struct {
	g.Meta  `path:"developer/codestore/savePackName" tags:"savePackName" method:"post" summary:"检查插件标识是否可用"`
	Baseurl string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Name    string `p:"name" v:"required#标识不能为空" dc:"标识"`
	Type    string `p:"type" v:"required#类型不能为空" dc:"类型"`
}
type SavePackNameRes struct {
	*gf.R
}

// 发布需求
type RequirementReq struct {
	g.Meta    `path:"developer/codestore/requirement" tags:"requirement" method:"post" summary:"发布需求"`
	Baseurl   string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Title     string `p:"title" v:"required#标题不能为空" dc:"标题"`
	CodeToken string `p:"code_token" v:"required#请登录后再提交" dc:"认证用户身份"`
	Cid       int64  `p:"cid" d:"11" dc:"分类"`
	Type      int64  `p:"type" d:"1" dc:"类型"`
	Status    int64  `p:"status" d:"2" dc:"状态"`
	FrameType int64  `p:"frame_type" d:"goframe" dc:"发布来自"`
	Des       string `p:"des" v:"required#说明不能为空" dc:"说明"`
	Name      string `p:"name" v:"required#包名不能为空" dc:"限定包名"`
	Price     string `p:"price" v:"required#金额不能为空" dc:"金额"`
	Customer  string `p:"customer" v:"required#联系人不能为空" dc:"联系人"`
	Mobile    string `p:"mobile" v:"required#联系电话不能为空" dc:"联系电话"`
	Wx        string `p:"wx" dc:"微信"`
	Content   string `p:"content" dc:"内容"`
}
type RequirementRes struct {
	*gf.R
}

// 发布代码插件到代码仓
type UpPackToServiceReq struct {
	g.Meta      `path:"developer/codestore/upPackToService" tags:"upPackToService" method:"post" summary:"发布代码插件到代码仓"`
	Baseurl     string  `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Title       string  `p:"title" v:"required#包名不能为空" dc:"代码包名"`
	Name        string  `p:"name" v:"required#名称不能为空" dc:"代码名称"`
	Version     string  `p:"version" v:"required#版本号不能为空" dc:"版本号"`
	Des         string  `p:"des" d:"" dc:"描述"`
	Price       float64 `p:"price" dc:"价格"`
	Cid         int64   `p:"cid" dc:"分类"`
	Frame       int64   `p:"frame" d:"2" dc:"插件框架类型"`
	CodeToken   string  `p:"code_token" dc:"用户身份标识"`
	GoframeFile string  `p:"goframe_file" v:"required#插件代码文件不能为空" dc:"代码文件"`
}
type UpPackToServiceRes struct {
	*gf.R
}

/********************插件打包******************************/
//获取文件路径
type GetPackdirsReq struct {
	g.Meta `path:"developer/codestore/getPackdirs" tags:"getPackdirs" method:"get" summary:"获取文件路径"`
	Type   string `p:"type" d:"vue" dc:"目录类型"`
}
type GetPackdirsRes struct {
	*gf.R
}

// 获取后台菜单
type GetMenutreeReq struct {
	g.Meta `path:"developer/codestore/getMenutree" tags:"getMenutree" method:"get" summary:"获取后台菜单"`
}
type GetMenutreeRes struct {
	*gf.R
}

// 菜单id转JSON数据
type MenuTreeToJsonReq struct {
	g.Meta `path:"developer/codestore/menuTreeToJson" tags:"menuTreeToJson" method:"post" summary:"菜单id转JSON数据"`
	Menu   g.Slice `p:"menu" d:"[]" dc:"选择的菜单id"`
}
type MenuTreeToJsonRes struct {
	*gf.R
}

// 获取邮箱验证码
type LoginCodeReq struct {
	g.Meta  `path:"developer/codestore/loginCode" tags:"loginCode" method:"post" summary:"获取邮箱验证码"`
	Baseurl string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Email   string `p:"email" v:"required|email#邮箱不能为空|邮箱格式不正确" dc:"邮箱账号"`
}
type LoginCodeRes struct {
	*gf.R
}

// 免密登录
type FreeLoginReq struct {
	g.Meta     `path:"developer/codestore/freeLogin" tags:"freeLogin" method:"post" summary:"免密登录"`
	Baseurl    string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Email      string `p:"email" v:"required|email#邮箱不能为空|邮箱格式不正确" dc:"邮箱账号"`
	Verifycode string `p:"verifycode" v:"required#验证码不能为空" dc:"验证码"`
}
type FreeLoginRes struct {
	*gf.R
}

// 注册账号
type RegisterUserReq struct {
	g.Meta   `path:"developer/codestore/registerUser" tags:"registerUser" method:"post" summary:"免密登录"`
	Baseurl  string `p:"baseurl" v:"required#代码仓地址不能为空" dc:"代码仓地址"`
	Email    string `p:"email" v:"required|email#邮箱不能为空|邮箱格式不正确" dc:"邮箱账号"`
	Code     string `p:"code" v:"required#验证码不能为空" dc:"验证码"`
	Mobile   string `p:"mobile" d:"" v:"telephone#手机号格式不正确" dc:"手机号"`
	Password string `p:"password" v:"required#密码不能为空" dc:"密码"`
}
type RegisterUserRes struct {
	*gf.R
}
