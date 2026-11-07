package admininstall

import "github.com/suxinwl/GoSuxin/internal/service"

//1.定义接收器名称
type (
	sAdmininstall struct{}
)

//2.把sAdmininstall注册到service中
func NewAdmininstall() service.IAdmininstall {
	return &sAdmininstall{}
}

func init() {
	service.RegisterAdmininstall(NewAdmininstall())
}
