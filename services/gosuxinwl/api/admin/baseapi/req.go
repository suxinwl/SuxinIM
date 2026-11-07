// ===================
// 响应公共参数，如：分页
// ====================
package baseapi

// PageReq  请求参数：页码、排序
type PageReq struct {
	Page     int    `p:"page" d:"1"`      //当前页码
	PageSize int    `p:"pageSize" d:"10"` //每页数
	Sort     string `p:"sort" d:"desc"`   // 排序方式
}
