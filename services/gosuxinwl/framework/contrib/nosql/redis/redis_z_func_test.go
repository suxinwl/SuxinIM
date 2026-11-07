// Copyright GoFrame Author(https://goframe.org). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package redis

import (
	"testing"

	"github.com/suxinwl/GoSuxin/framework/database/gredis"
	"github.com/suxinwl/GoSuxin/framework/test/gtest"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

func Test_mustMergeOptionToArgs(t *testing.T) {
	gtest.C(t, func(t *gtest.T) {
		var args []any
		newArgs := mustMergeOptionToArgs(args, gredis.SetOption{
			NX:  true,
			Get: true,
		})
		t.Assert(newArgs, []any{"NX", "Get"})
	})
	gtest.C(t, func(t *gtest.T) {
		var args []any
		newArgs := mustMergeOptionToArgs(args, gredis.SetOption{
			NX:  true,
			Get: true,
			TTLOption: gredis.TTLOption{
				EX: gconv.PtrInt64(60),
			},
		})
		t.Assert(newArgs, []any{"EX", 60, "NX", "Get"})
	})
}
