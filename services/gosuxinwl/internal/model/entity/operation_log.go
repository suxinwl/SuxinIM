// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package entity

import (
	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// OperationLog is the golang structure for table operation_log.
type OperationLog struct {
	Id          uint        `json:"id"          orm:"id"           description:""`
	Uid         int         `json:"uid"         orm:"uid"          description:"用户id"`
	Method      string      `json:"method"      orm:"method"       description:"请求方法"`
	Url         string      `json:"url"         orm:"url"          description:"请求地址"`
	Ip          string      `json:"ip"          orm:"ip"           description:"登录IP"`
	Address     string      `json:"address"     orm:"address"      description:"地点"`
	Des         string      `json:"des"         orm:"des"          description:"登录行为"`
	ReqHeaders  string      `json:"reqHeaders"  orm:"req_headers"  description:"请求头"`
	ReqBody     string      `json:"reqBody"     orm:"req_body"     description:"请求体"`
	RespHeaders string      `json:"respHeaders" orm:"resp_headers" description:"响应头"`
	RespBody    string      `json:"respBody"    orm:"resp_body"    description:"响应体"`
	Latency     float64     `json:"latency"     orm:"latency"      description:"耗时"`
	Status      int         `json:"status"      orm:"status"       description:"状态:0=成功,1=失败"`
	Createtime  *gtime.Time `json:"createtime"  orm:"createtime"   description:"创建时间"`
}
