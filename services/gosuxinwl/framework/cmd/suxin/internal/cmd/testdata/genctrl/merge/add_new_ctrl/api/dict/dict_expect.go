// =================================================================================
// Code generated and maintained by Suxin CLI tool. DO NOT EDIT.
// =================================================================================

package dict

import (
	"context"

	"github.com/suxinwl/GoSuxin/framework/cmd/suxin/internal/cmd/testdata/genctrl/merge/add_new_ctrl/api/dict/v1"
)

type IDictV1 interface {
	DictTypeAddPage(ctx context.Context, req *v1.DictTypeAddPageReq) (res *v1.DictTypeAddPageRes, err error)
}
