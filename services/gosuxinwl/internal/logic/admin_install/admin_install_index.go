package admininstall

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/api/admin/install"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/suxinwl/GoSuxin/framework/encoding/gcompress"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/text/gstr"
)

var installMu sync.Mutex

// 获取安装配置数据
func (s *sAdmininstall) GetConfig(ctx context.Context, req *install.GetConfigReq) (res *gf.R) {
	if _, err := os.Stat("devsource/developer/install/install.lock"); err == nil {
		return gf.Success().SetData(gf.Map{"isLock": true})
	}
	database, _ := g.Cfg().Get(ctx, "database.default")
	mapDatabase := database.Map()
	kind := "mysql8"
	if mapDatabase["type"] == "sqlite" {
		kind = "sqlite"
	} else if mapDatabase["version"] == "5.7" {
		kind = "mysql57"
	}
	sqlitePath := "storage/database/gosuxin.db"
	if kind == "sqlite" {
		sqlitePath = fmt.Sprint(mapDatabase["name"])
	}
	appConf, _ := g.Cfg("app").Get(ctx, "app")
	mapAppConf := appConf.Map()
	path, _ := os.Getwd() //获取当前路径
	filePath := filepath.Join(path, "/devsource/developer/install/install.lock")
	_, errLock := os.Stat(filePath)
	res = gf.Success().SetMsg("获取应用配置").SetData(gf.Map{
		"database": gf.Map{
			"type":   kind,
			"path":   sqlitePath,
			"host":   mapDatabase["host"],
			"port":   mapDatabase["port"],
			"user":   mapDatabase["user"],
			"pass":   "",
			"name":   mapDatabase["name"],
			"prefix": mapDatabase["prefix"],
		},
		"app": gf.Map{
			"vueobjroot": mapAppConf["vueobjroot"],
		},
		"isLock": errLock == nil,
	})
	return
}

// 执行安装操作
func (s *sAdmininstall) Save(ctx context.Context, req *install.SaveReq) (res *gf.R) {
	installMu.Lock()
	defer installMu.Unlock()
	if _, err := os.Stat("devsource/developer/install/install.lock"); err == nil {
		return gf.Failed().SetMsg("项目已安装，禁止重复安装")
	}
	values, err := normalizeDatabase(req.FormDB)
	if err != nil {
		return gf.Failed().SetMsg(err.Error())
	}
	if strings.TrimSpace(req.FormApp["username"]) == "" || req.FormApp["password"] == "" {
		return gf.Failed().SetMsg("请填写管理员账号和密码")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	db, err := openInstallDatabase(ctx, values)
	if err != nil {
		return gf.Failed().SetMsg("数据库检查失败：" + err.Error())
	}
	defer db.Close()
	//2.修改app配置
	if !g.IsEmpty(req.FormApp["vueobjroot"]) { //判断目录是否有其他文件
		dir, _ := os.ReadDir(filepath.Join(req.FormApp["vueobjroot"]))
		if len(dir) > 0 && !strings.Contains(req.FormApp["vueobjroot"], "gosuxin-web") { //如果存在文件创建新目录
			req.FormApp["vueobjroot"] = filepath.Join(req.FormApp["vueobjroot"]) + "/gosuxin-web"
		}
	}
	apperr := gf.UpConfigFild("/manifest/config/app.yaml", gf.Map{
		"vueobjroot": gstr.Replace(req.FormApp["vueobjroot"], "\\", "/"),
	}, "  ")
	if apperr != nil {
		res = gf.Failed().SetMsg("修改app配置失败！").SetData(apperr.Error())
		return
	}

	//3.安装前端代码
	path, err := os.Getwd() //获取当前路径
	if err != nil {
		res = gf.Failed().SetMsg("项目路径获取失败").SetData(err.Error())
		return
	}
	//前端zip代码包
	srcFilePath := filepath.Join(path, "/devsource/developer/install/webcode.zip")
	dstFilePath := filepath.Join(path, "/web")
	if !g.IsEmpty(req.FormApp["vueobjroot"]) {
		dstFilePath = filepath.Join(req.FormApp["vueobjroot"])
	}
	if unzipErr := gcompress.UnZipFile(srcFilePath, dstFilePath, "webcode"); unzipErr != nil {
		return gf.Failed().SetMsg("释放前端安装包失败")
	}
	if err = seedDatabase(ctx, db, values["type"], filepath.Join(path, "devsource/developer/install"), req.FormApp["username"], req.FormApp["password"]); err != nil {
		return gf.Failed().SetMsg("数据库初始化失败：" + err.Error())
	}
	if err = writeDatabaseConfig("manifest/config/config.yaml", values); err != nil {
		return gf.Failed().SetMsg("保存数据库配置失败：" + err.Error())
	}
	if err = writeCLIConfig("hack/config.yaml", values); err != nil {
		return gf.Failed().SetMsg("保存开发工具配置失败：" + err.Error())
	}
	//7.创建安装锁文件-防止重复安装
	filePath := filepath.Join(path, "/devsource/developer/install/install.lock")
	if err = os.WriteFile(filePath, []byte("installed\n"), 0600); err != nil {
		return gf.Failed().SetMsg("创建安装锁失败：" + err.Error())
	}
	res = gf.Success().SetMsg("安装成功，请重启后端服务后登录").SetData(path)
	return
}
