package common

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/utility/dbcompat"

	"github.com/suxinwl/GoSuxin/api/common/basetool"
	"github.com/suxinwl/GoSuxin/utility/gf"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
)

// 数据表排序
func (c *ControllerBasetool) Weigh(ctx context.Context, req *basetool.WeighReq) (res *basetool.WeighRes, err error) {
	var resData = gf.Failed()
	// Sorting only updates existing IDs; upsert would insert incomplete rows.
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, row := range gconv.Maps(req.WeighList) {
			id := gconv.Int64(row["id"])
			if id <= 0 {
				return fmt.Errorf("排序记录缺少有效 id")
			}
			if _, exists := row["weigh"]; !exists {
				return fmt.Errorf("排序记录缺少 weigh")
			}
			if _, err := tx.Model(req.Tableanme).Where("id", id).Update(g.Map{"weigh": gconv.Int(row["weigh"])}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		resData = gf.Failed().SetMsg("排序更新失败！").SetData(err)
	} else {
		resData = gf.Success().SetMsg("排序更新成功！").SetData(res)
	}
	res = &basetool.WeighRes{
		R: resData,
	}
	return
}

// 获取数据表
func (c *ControllerBasetool) GetTables(ctx context.Context, req *basetool.GetTablesReq) (res *basetool.GetTablesRes, err error) {
	rows, queryErr := dbcompat.Tables(ctx, g.DB())
	result := gf.Success().SetData(rows)
	if queryErr != nil {
		result = gf.Failed().SetMsg("获取数据表失败").SetData(queryErr.Error())
	}
	return &basetool.GetTablesRes{R: result}, nil
}
