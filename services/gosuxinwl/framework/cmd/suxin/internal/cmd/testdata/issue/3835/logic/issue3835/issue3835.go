package issue3835

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/issue/3835/service"

	"github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
)

func init() {
	service.RegisterItest(New())
}

type sItest struct {
}

func New() *sItest {
	return &sItest{}
}

func (s *sItest) F(ctx context.Context) (d mysql.Driver, err error) {
	return mysql.Driver{}, nil
}
