package gf

import (
	"encoding/json"

	"github.com/suxinwl/GoSuxin/framework/os/gtime"
)

// 统一返回接口请求数据
type R struct {
	Code    int         `json:"code"    dc:"Error code"`
	Message string      `json:"message" dc:"Error message"`
	Data    interface{} `json:"data"    dc:"Result data for certain request according API definition"`
	Exdata  interface{} `json:"exdata"  dc:"Return the extended data auxiliary data"`
	Time    int64       `json:"time"    dc:"Request data return time"`
}

// 错误码
var (
	succCode = 0 // 成功
	errCode  = 1 // 失败
)

// 设置编码
func (r *R) SetCode(code int) *R {
	r.Code = code
	return r
}

// 设置返回提示信息
func (r *R) SetMsg(msg string) *R {
	r.Message = msg
	return r
}

// 设置返回内容
func (r *R) SetData(data interface{}) *R {
	r.Data = data
	return r
}

// 设置返回扩展内容(开发返回测数据-很实用)
func (r *R) SetExdata(exdata interface{}) *R {
	r.Exdata = exdata
	return r
}

// 返回成功内容
func Success() *R {
	r := &R{}
	r.Message = "Success"
	r.Code = succCode
	r.Time = gtime.Now().UnixMilli()
	return r
}

// 返回失败内容
func Failed() *R {
	r := &R{}
	r.Message = "Fail"
	r.Code = errCode
	r.Data = false
	r.Time = gtime.Now().UnixMilli()
	return r
}

// 序列化为json字符串
func (r *R) ToJson() []byte {
	bty, _ := json.Marshal(r.Data)
	str := string(bty)
	r.SetData(str)
	rr, _ := json.Marshal(r)
	return rr
}

// json字符串序列化为R对象
func StoR(jsonStr string) *R {
	return BtoR(([]byte)(jsonStr))
}
func BtoR(bty []byte) *R {
	var r *R
	err := json.Unmarshal(bty, &r)
	if nil != err {
		return nil
	}
	return r
}

func (r *R) Ok() bool {
	if succCode == r.Code {
		return true
	}
	return false
}
