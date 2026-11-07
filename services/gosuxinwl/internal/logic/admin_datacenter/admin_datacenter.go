package admindatacenter

import "github.com/suxinwl/GoSuxin/internal/service"

//1.定义接收器名称
type (
	sAdmindatacenter struct{}
)

//2.把sAdmindatacenter注册到service中
func NewAdmindatacenter() service.IAdmindatacenter {
	return &sAdmindatacenter{}
}

func init() {
	service.RegisterAdmindatacenter(NewAdmindatacenter())
}
