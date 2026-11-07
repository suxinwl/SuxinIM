// Copyright GoFrame gf Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package cmd

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

var Up = cUp{}

type cUp struct {
	g.Meta `name:"up" brief:"online upgrades are unavailable; install an explicit published Suxin version"`
}
type cUpInput struct {
	g.Meta               `name:"up" config:"suxincli.up"`
	All                  bool   `name:"all" short:"a" brief:"upgrade both version and cli, auto fix codes" orphan:"true"`
	Cli                  bool   `name:"cli" short:"c" brief:"also upgrade CLI tool" orphan:"true"`
	Fix                  bool   `name:"fix" short:"f" brief:"auto fix codes(it only make sense if cli is to be upgraded)" orphan:"true"`
	CliDownloadingMethod string `name:"cli-download-method" short:"m" brief:"cli upgrade method: http=download binary via HTTP GET, install=upgrade via go install" d:"http"`
	// CliModulePath specifies the module path for CLI installation via go install.
	// This is used when CliDownloadingMethod is set to "install".
	CliModulePath string `name:"cli-module-path" short:"p" brief:"custom cli module path for upgrade CLI tool with go install method" d:"github.com/suxinwl/GoSuxin/framework/cmd/suxin@latest"`
}
type cUpOutput struct{}

func (c cUp) Index(ctx context.Context, in cUpInput) (*cUpOutput, error) {
	return nil, fmt.Errorf("online upgrades are unavailable; install an explicit published Suxin version")
}

const gfPackage = "github.com/suxinwl/GoSuxin/framework"
