package excelexport

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
	"github.com/suxinwl/GoSuxin/utility/dbcompat"
	"github.com/xuri/excelize/v2"
)

// ExportToExcel 导出数据库记录为 Excel 文件流
// rows数据，columns导出表字段，tableName表名称,fileName导出文件名（为空时是默认时间命名）
func ExportToExcel(ctx context.Context, rows *gdb.Result, columns []interface{}, tableName, fileName string) ([]byte, error) {
	// 创建 Excel 文件
	file := excelize.NewFile()
	defer file.Close()
	sheet := "Sheet1"
	// 获取列名和列注释
	var fields []string
	var columnComments []string
	if rows != nil && len(*rows) > 0 {
		if len(columns) > 0 {
			// 按照查询结果的顺序存储列名和列注释
			for _, column := range columns {
				columndata, ok := column.(map[string]interface{})
				if !ok {
					return nil, fmt.Errorf("无效的导出字段: %T", column)
				}
				colName := gconv.String(columndata["field"])
				colComment := gconv.String(columndata["title"])
				if colComment == "" {
					colComment = colName
				}
				fields = append(fields, colName)
				columnComments = append(columnComments, colComment)
			}
		} else {
			// 查询数据库获取字段注释
			fieldData, err := dbcompat.Columns(ctx, g.DB(), tableName)
			if err != nil {
				return nil, fmt.Errorf("查询注释失败: %v", err)
			}

			// 按照查询结果的顺序存储列名和列注释
			for _, data := range fieldData {
				colName := data.Name
				colComment := data.Comment
				if colComment == "" {
					colComment = colName
				}
				fields = append(fields, colName)
				columnComments = append(columnComments, colComment)
			}
		}

	} else {
		return nil, fmt.Errorf("没有数据")
	}

	// 写入表头（使用列注释作为表头）
	columnNames := make([]string, len(columnComments))
	for i, col := range columnComments {
		name, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return nil, err
		}
		columnNames[i] = name
		if err := file.SetCellValue(sheet, name+"1", col); err != nil {
			return nil, err
		}
	}

	// 写入数据
	rowIndex := 2
	for _, row := range *rows {
		for i, col := range fields {
			val := row[col]
			if err := file.SetCellValue(sheet, columnNames[i]+strconv.Itoa(rowIndex), val.Val()); err != nil {
				return nil, err
			}
		}
		rowIndex++
	}

	// 将 Excel 文件写入缓冲区
	buf, err := file.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("将 Excel 文件写入缓冲区出错: %w", err)
	}
	if g.IsEmpty(fileName) {
		fileName = url.QueryEscape(fmt.Sprintf("%s.xlsx", time.Now().Format("20060102150405"))) // 中文编码处理
	} else {
		fileName = url.QueryEscape(fmt.Sprintf("%s.xlsx", fileName)) // 中文编码处理
	}
	req := g.RequestFromCtx(ctx)
	if req != nil {
		req.Response.Writer.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		req.Response.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
		req.Response.Writer.Header().Set("Content-Transfer-Encoding", "binary")
		req.Response.Writer.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
		req.Response.Writer.Header().Set("Content-Length", strconv.Itoa(buf.Len())) // 添加文件大小标识
		if _, err := req.Response.Writer.Write(buf.Bytes()); err != nil {
			return nil, err
		}
	}
	// 返回文件流
	return buf.Bytes(), nil
}
