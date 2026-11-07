package adminsystem

import "github.com/suxinwl/GoSuxin/internal/service"

type (
	sAdminsystem struct{}
)

//把sAdminsystem注册到service中
func NewAdminUser() service.IAdminsystem {
	return &sAdminsystem{}
}

func init() {
	service.RegisterAdminsystem(NewAdminUser())
}
