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

var Doc = cDoc{}

type cDoc struct {
	g.Meta `name:"doc" brief:"online documentation hosting is pending; see README.MD in the source distribution and suxin --help"`
}
type cDocInput struct {
	g.Meta `name:"doc" config:"suxincli.doc"`
	Path   string `short:"p"  name:"path"    brief:"download docs directory path, default is \"%temp%/suxin\""`
	Port   int    `short:"o"  name:"port"    brief:"http server port, default is 8080" d:"8080"`
	Update bool   `short:"u"  name:"update"  brief:"clean docs directory and update docs"`
	Clean  bool   `short:"c"  name:"clean"   brief:"clean docs directory"`
	Proxy  string `short:"x"  name:"proxy"   brief:"proxy for download, such as https://hub.gitmirror.com/;https://ghproxy.com/;https://ghproxy.net/;https://ghps.cc/"`
}
type cDocOutput struct{}

func (c cDoc) Index(ctx context.Context, in cDocInput) (*cDocOutput, error) {
	return nil, fmt.Errorf("online documentation hosting is pending; see README.MD in the source distribution and suxin --help")
}
