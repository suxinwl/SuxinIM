package admindeveloper

import "github.com/suxinwl/GoSuxin/internal/service"

//1.定义接收器名称
type (
	sAdmindeveloper struct{}
)

//2.把sAdmindeveloper注册到service中
func NewAdmindeveloper() service.IAdmindeveloper {
	return &sAdmindeveloper{}
}

func init() {
	service.RegisterAdmindeveloper(NewAdmindeveloper())
}
