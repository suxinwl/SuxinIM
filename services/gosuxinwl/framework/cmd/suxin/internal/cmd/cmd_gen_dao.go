// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package cmd

import (
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/clickhouse"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mssql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/mysql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/oracle"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/pgsql"
	_ "github.com/suxinwl/GoSuxin/framework/contrib/drivers/sqlite"

	// do not add dm in cli pre-compilation,
	// the dm driver does not support certain target platforms.
	// _ "github.com/gogf/gf/contrib/drivers/dm/v2"
	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/gendao"
)

type (
	cGenDao = gendao.CGenDao
)
