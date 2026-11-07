package clogic

import (
	"context"
	"github.com/suxinwl/GoSuxin/internal/dao"
)

// 添加授权
func AppRoleAccess(ctx context.Context, roleids []interface{}, uid interface{}) {
	//批量提交
	dao.AuthRoleAccess.Ctx(ctx).Where("uid", uid).Delete()
	save_arr := []map[string]interface{}{}
	for _, val := range roleids {
		marr := map[string]interface{}{"uid": uid, "role_id": val}
		save_arr = append(save_arr, marr)
	}
	if len(save_arr) > 0 {
		dao.AuthRoleAccess.Ctx(ctx).Data(save_arr).Insert()
	}
}
