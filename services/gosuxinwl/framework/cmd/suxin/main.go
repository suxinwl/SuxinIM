// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package main

import (
	_ "time/tzdata"

	"github.com/suxinwl/GoSuxin/framework/errors/gerror"
	"github.com/suxinwl/GoSuxin/framework/os/gctx"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/gfcmd"
	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/utility/mlog"
)

func main() {
	var (
		ctx          = gctx.GetInitCtx()
		command, err = gfcmd.GetCommand(ctx)
	)

	if err != nil {
		mlog.Fatalf(`%+v`, err)
	}
	if command == nil {
		panic(gerror.New(`retrieve root command failed for "gf"`))
	}
	command.Run(ctx)
}
