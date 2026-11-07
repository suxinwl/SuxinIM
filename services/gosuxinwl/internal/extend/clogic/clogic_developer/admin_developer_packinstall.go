// =================
// 开发工具-插件打包、安装、卸载
// =================
package clogic_developer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/utility/tools/dbtool"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/text/gstr"
)

// 更新配置文件
func UpConfFieldData(path string, parameter map[string]interface{}) error {
	file_path := filepath.Join(path, "config.yml")
	f, err := os.Open(file_path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var result = ""
	var is_hose = false
	for {
		is_hose = false
		a, _, c := buf.ReadLine()
		if c == io.EOF {
			break
		}
		for keys, Val := range parameter {
			if strings.Contains(string(a), keys) {
				is_hose = true
				datestr := strings.ReplaceAll(string(a), string(a), fmt.Sprintf("     %v: %v\n", keys, Val))
				result += datestr
			}
		}
		if !is_hose {
			result += string(a) + "\n"
		}
	}
	fw, err := os.OpenFile(file_path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666) //os.O_TRUNC清空文件重新写入，否则原文件内容可能残留
	w := bufio.NewWriter(fw)
	w.WriteString(result)
	if err != nil {
		panic(err)
	}
	w.Flush()
	fw.Close()
	return nil
}

// 导出数据库数据sql文件
func ExecSqlFile(tables []string, pathname string) {
	f, _ := os.Create(pathname)
	_ = dbtool.DBDump(
		dbtool.WithDropTable(),    // Option: Delete table before create (Default: Not delete table)
		dbtool.WithData(),         // Option: Dump Data (Default: Only dump table schema)
		dbtool.WithTables(tables), // Option: Dump Tables (Default: All tables)
		dbtool.WithWriter(f),      // Option: Writer (Default: os.Stdout)
	)
	f.Close()
}

// 导入sql文件
func ImportSqlFile(SqlPath string) error {
	return dbtool.ImportSql(SqlPath)
}

// UpFileClient 上传文件
func UpFileClient(c *developer.UpfileReq, params map[string]string) ([]byte, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	// 其他参数列表写入 body
	for k, v := range params {
		if err := writer.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	// 一个是输入表单的 name，一个上传的文件名称
	uploadWriter, _ := writer.CreateFormFile("file", c.File.Filename)
	uploadFile, err := c.File.Open()
	if err != nil {
		return nil, err
	}
	defer uploadFile.Close()
	_, err = io.Copy(uploadWriter, uploadFile)
	if err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	resp, err := http.Post(c.Domainurl+"/goflycode/upfile/codeFile",
		writer.FormDataContentType(),
		body,
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	return content, err
}

// 下载文件到指定目录 url下载地址 downdir 下载到位置及文件名称
func DownFileToDir(url, downdir string) (bool, string) {
	// 获取网络文件的数据流
	resp, err := http.Get(url)
	if err != nil {
		// 处理错误
		return false, ""
	}
	defer resp.Body.Close()
	// 读取数据流到内存中
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, ""
	}
	// 将文件写入到指定目录
	err = os.WriteFile(downdir, data, 0644)
	if err != nil {
		return false, ""
	}
	return true, downdir
}

// 导入菜单数据、并返回插入数据id
func Insertmenu(userID int64, data interface{}, pid interface{}, tablename string) []string {
	var menuids = make([]string, 0)
	for _, menuitem := range data.([]interface{}) {
		menuitem_obj := menuitem.(map[string]interface{})
		menuitem_obj["pid"] = pid
		menuitem_obj["uid"] = userID
		delete(menuitem_obj, "id")
		parent_id, _ := g.Model(tablename).Where("pid", 0).Where("routename", menuitem_obj["routename"]).Value("id")
		if parent_id == nil {
			if _, ok := menuitem_obj["children"]; ok {
				subdata := menuitem_obj["children"]
				delete(menuitem_obj, "children")
				nemuid, _ := g.Model(tablename).Data(menuitem_obj).InsertAndGetId()
				g.Model(tablename).Where("id", nemuid).Data(map[string]interface{}{"weigh": nemuid}).Update()
				menuids = append(menuids, strconv.FormatInt(nemuid, 10))
				if subdata != nil {
					m_menuids := Insertmenu(userID, subdata, nemuid, tablename)
					menuids = append(menuids, m_menuids...)
				}
			} else {
				hase_nemuid, err := g.Model(tablename).Where("routepath", menuitem_obj["routepath"]).Where("routename", menuitem_obj["routename"]).Value("id")
				if err != nil || hase_nemuid == nil {
					nemuid, _ := g.Model(tablename).Data(menuitem_obj).InsertAndGetId()
					g.Model(tablename).Where("id", nemuid).Data(map[string]interface{}{"weigh": nemuid}).Update()
					menuids = append(menuids, strconv.FormatInt(nemuid, 10))
				} else {
					menuids = append(menuids, hase_nemuid.String())
				}
			}
		} else {
			subdata := menuitem_obj["children"]
			if subdata != nil {
				m_menuids := Insertmenu(userID, subdata, parent_id, tablename)
				menuids = append(menuids, m_menuids...)
			}
		}
	}
	return menuids
}

//	处理路由注册(模块路由，功能路由)
//
// modelName文件名称使用
// funName 变量名使用
func RouteRegister(modelName, funName, path string) {
	//功能路由(小)
	CheckIsAddCtrRouter(filepath.Join(path, "internal/controller", modelName, modelName+"_router.go"), fmt.Sprintf("New%v(),", gstr.UcFirst(funName)), "ctr")
	//模块路由(大)
	CheckIsAddCtrRouter(filepath.Join(path, "internal/router", "router.go"), modelName, "model")
}

// 检查是否注册路由，否则添加
func CheckIsAddCtrRouter(routerPath, routerName, routerType string) error {
	//1判断文件没有则添加
	if _, err := os.Stat(routerPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("该模块路由文件不存在")
		}
	}
	//打开路由文件
	f, err := os.Open(routerPath)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var result = ""
	ishase := true
	for {
		a, _, c := buf.ReadLine()
		if c == io.EOF {
			break
		}
		result += string(a) + "\n"
		//判断控制器内容是否存在要引入的模块
		if routerType == "ctr" && strings.Contains(string(a), routerName) {
			ishase = false
		} else if routerType == "model" && strings.Contains(string(a), routerName+".R.BindController(ctx, group)") {
			ishase = false
		}
	}
	if ishase {
		if routerType == "ctr" {
			addstr := "\t" + routerName + "\n"
			addstr += "\t\t) //append"
			result = strings.Replace(result, ") //append", addstr, 1)
		} else if routerType == "model" {
			//添加引入
			iaddstr := "\t\"github.com/suxinwl/GoSuxin/internal/controller/" + routerName + "\"\n"
			iaddstr += ") //append"
			result = strings.Replace(result, ") //append", iaddstr, 1)
			//添加路由
			addstr := "\t" + routerName + ".R.BindController(ctx, group)\n"
			addstr += "\t}) //append"
			result = strings.Replace(result, "}) //append", addstr, 1)
		}
	}

	fw, err := os.OpenFile(routerPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666) //os.O_TRUNC清空文件重新写入，否则原文件内容可能残留
	w := bufio.NewWriter(fw)
	w.WriteString(result)
	if err != nil {
		return err
	}
	w.Flush()
	fw.Close()
	return nil
}

/****************************卸载******************************/
//删除全部路由
func DelRouter(modelName, funName, path string) {
	//删除功能路由
	DelFunRouter(modelName, funName, path)
	//删除new文件
	UnCtNew(filepath.Join(path, "internal/controller", modelName, modelName+"_new.go"), gstr.UcFirst(funName))
	//删除模块路由
	DelModelRouter(modelName, path)
}

// 删除功能路由
func DelFunRouter(modelName, funName, path string) {
	//功能路由(判断模块是否还存在其他功能文件)
	funFilePath := filepath.Join(path, "/api", modelName, funName)
	if _, err := os.Stat(funFilePath); err != nil && os.IsNotExist(err) { //不存在
		UnCtrRouter(filepath.Join(path, "internal/controller", modelName, modelName+"_router.go"), fmt.Sprintf("New%v(),", gstr.UcFirst(funName)), "ctr")
	}
}

// 删除模块路由
func DelModelRouter(modelName, path string) {
	//模块路由(判断模块是否还存在其他功能文件)
	staticFilePath := filepath.Join(path, "/api", modelName)
	if _, err := os.Stat(staticFilePath); err != nil && os.IsNotExist(err) { //不存在
		UnCtrRouter(filepath.Join(path, "internal/router", "router.go"), modelName, "model")
	}
}
func UnCtrRouter(routerPath, routerName, routerType string) error {
	//1判断文件没有则添加
	if _, err := os.Stat(routerPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("该模块路由文件不存在")
		}
	}
	//打开路由文件
	f, err := os.Open(routerPath)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var result = ""
	for {
		a, _, c := buf.ReadLine()
		if c == io.EOF {
			break
		}
		if routerType == "ctr" && strings.Contains(string(a), routerName) {
			continue
		} else if routerType == "model" && strings.Contains(string(a), "github.com/suxinwl/GoSuxin/internal/controller/"+routerName) {
			continue
		} else if routerType == "model" && strings.Contains(string(a), routerName+".R.BindController(ctx, group)") {
			continue
		} else {
			result += string(a) + "\n"
		}
	}

	fw, err := os.OpenFile(routerPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666) //os.O_TRUNC清空文件重新写入，否则原文件内容可能残留
	w := bufio.NewWriter(fw)
	w.WriteString(result)
	if err != nil {
		return err
	}
	w.Flush()
	fw.Close()
	return nil
}

// 删除模块new文件引入功能
func UnCtNew(routerPath, funName string) error {
	//1判断文件没有则添加
	if _, err := os.Stat(routerPath); err != nil {
		if os.IsNotExist(err) {
			return errors.New("该模块New文件不存在")
		}
	}
	//打开路由文件
	f, err := os.Open(routerPath)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var result = ""
	var delRow int64 = 0
	for {
		a, _, c := buf.ReadLine()
		if c == io.EOF {
			break
		}
		if delRow > 0 {
			delRow--
			continue
		} else {
			if strings.Contains(string(a), "type Controller"+funName+" struct{}") {
				delRow = 2
				continue
			} else if strings.Contains(string(a), "New"+funName+"()") {
				continue
			} else if strings.Contains(string(a), "&Controller"+funName+"{}") {
				delRow = 2
				continue
			} else {
				if g.IsEmpty(result) {
					result += string(a)
				} else {
					result += "\n" + string(a)
				}
			}
		}

	}

	fw, err := os.OpenFile(routerPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666) //os.O_TRUNC清空文件重新写入，否则原文件内容可能残留
	w := bufio.NewWriter(fw)
	w.WriteString(result)
	if err != nil {
		return err
	}
	w.Flush()
	fw.Close()
	return nil
}
