package main

import (
	_ "github.com/suxinwl/GoSuxin/internal/packed"

	_ "github.com/suxinwl/GoSuxin/internal/logic"

	"github.com/suxinwl/GoSuxin/framework/os/gctx"

	"github.com/suxinwl/GoSuxin/internal/cmd"

	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/nosql/redis"
)

func main() {
	cmd.Main.Run(gctx.GetInitCtx())
}
