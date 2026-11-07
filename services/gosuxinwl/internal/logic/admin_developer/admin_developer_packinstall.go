// ================================
// 代码插件打包、安装、上传
// ================================
package admindeveloper

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/suxinwl/GoSuxin/api/admin/developer"
	"github.com/suxinwl/GoSuxin/internal/extend/clogic/clogic_developer"
	"github.com/suxinwl/GoSuxin/internal/plugins"
	"github.com/suxinwl/GoSuxin/internal/runtimeplugin"
	"github.com/suxinwl/GoSuxin/utility/gf"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/encoding/gcompress"
	"github.com/suxinwl/GoSuxin/framework/encoding/gjson"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/os/gfile"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

// 打包插件
func (c *sAdmindeveloper) PackCode(ctx context.Context, req *developer.PackCodeReq) (res *gf.R) {
	runEnv, _ := g.Cfg("app").Get(ctx, "app.runEnv")
	if runEnv.String() == "release" {
		res = gf.Failed().SetMsg("生产环境禁止操作，请在开发环境下操作")
		return
	}
	if g.IsEmpty(req.Name) {
		res = gf.Failed().SetMsg("请填写插件包名")
		return
	}
	path, err := os.Getwd()
	if err != nil {
		res = gf.Failed().SetMsg("项目路径获取失败")
		return
	}
	pack_path := filepath.Join(path, "/devsource/codemarket/release/", req.Name)
	//1制作打包文件
	if _, err := os.Stat(pack_path); err != nil && !os.IsExist(err) {
		os.MkdirAll(pack_path, os.ModePerm)
	} else {
		os.RemoveAll(pack_path)
		os.MkdirAll(pack_path, os.ModePerm)
	}
	//2复制包模板
	gfile.Copy(filepath.Join(path, "/devsource/developer/codetpl/packcode"), pack_path)
	//3.导出数据库表
	var tables = make([]string, 0)
	if !g.IsEmpty(req.Packtables) {
		pathname := filepath.Join(path, "/devsource/codemarket/release", req.Name, "install.sql")
		tables = strings.Split(req.Packtables, ",")
		clogic_developer.ExecSqlFile(tables, pathname)
	}
	//4.更新基础配置
	//处理数据表前缀
	var packtablesStr = ""
	if len(tables) > 0 {
		prefix, _ := g.Cfg().Get(ctx, "database.default.prefix")
		var tables_noprefix = make([]string, 0)
		for _, val := range tables {
			tables_noprefix = append(tables_noprefix, strings.Replace(val, prefix.String(), "", 1))
		}
		packtablesStr = strings.Join(tables_noprefix, ",")
	}
	upconf := map[string]interface{}{"version": req.Version, "title": req.Title, "installcover": req.Installcover, "isModTidy": req.IsModTidy, "commandLines": fmt.Sprintf("\"%v\"", req.CommandLines), "des": req.Des, "name": req.Name,
		"packtables": packtablesStr, "goFiles": fmt.Sprintf("'%v'", gf.String(req.GoFiles)), "vueFiles": fmt.Sprintf("'%v'", gf.String(req.VueFiles))}
	clogic_developer.UpConfFieldData(path+"/devsource/codemarket/release/"+req.Name, upconf)
	//5.把后台菜单数据写入adminmenu.json文件
	if len(req.Menujson) > 0 {
		meni_json := filepath.Join(path, "/devsource/codemarket/release/", req.Name, "adminmenu.json")
		if _, err := os.Stat(meni_json); err != nil {
			if !os.IsExist(err) {
				os.MkdirAll(meni_json, os.ModePerm)
			}
		}
		menudata, _ := gjson.Marshal(req.Menujson)
		os.WriteFile(meni_json, menudata, 0777)
	}
	//6. 查看/manifest/config/code是否存在动态配置文件-存在则复制到包目录下
	confFilePath := filepath.Join(path, "/manifest/config/code", req.Name+".yaml")
	if _, err := os.Stat(confFilePath); !os.IsNotExist(err) { //存在
		gfile.CopyFile(confFilePath, filepath.Join(pack_path, req.Name+".yaml"))
	}
	//7. 查看/resource/static是否存在静态文件-存在则复制到包目录下
	staticFilePath := filepath.Join(path, "/resource/static", req.Name)
	if _, err := os.Stat(staticFilePath); !os.IsNotExist(err) { //存在
		gfile.Copy(staticFilePath, filepath.Join(pack_path, req.Name))
	}
	//8.把后端(Go)代码复制到插件包中
	if !g.IsEmpty(req.GoFiles) {
		for _, item := range req.GoFiles {
			gfile.Copy(filepath.Join(path, gf.String(item["path"])), filepath.Join(path, "/devsource/codemarket/release/", req.Name, "/go", gf.String(item["path"])))
		}
	}
	//8.把前端(vue)代码复制到插件包中
	if !g.IsEmpty(req.VueFiles) {
		vueobjroot, _ := g.Cfg("app").Get(ctx, "app.vueobjroot")
		for _, item := range req.VueFiles {
			gfile.Copy(filepath.Join(vueobjroot.String(), gf.String(item["path"])), filepath.Join(path, "/devsource/codemarket/release/", req.Name, "/vue", gf.String(item["path"])))
		}
	}
	//打包文件路径
	if _, err := os.Stat(pack_path); err == nil {
		defer os.RemoveAll(pack_path) //最后删除文件夹
		dest := filepath.Join(path, "/devsource/codemarket/release", req.Name+".zip")
		err = gcompress.ZipPath(pack_path, dest)
		if err != nil {
			res = gf.Failed().SetMsg("打包压缩成zip错误").SetData(err)
			return
		}
		res = gf.Success().SetMsg("打包成功").SetData(true)
	} else {
		res = gf.Failed().SetMsg("文件不存在")
	}
	return
}

// 上传文件到代码仓
func (c *sAdmindeveloper) Upfile(ctx context.Context, req *developer.UpfileReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	params := map[string]string{"code_token": req.CodeToken}
	result, err := clogic_developer.UpFileClient(req, params)
	if err != nil {
		res = gf.Failed().SetMsg(err.Error())
		return
	}
	var parameter map[string]interface{}
	if err := json.Unmarshal([]byte(string(result)), &parameter); err == nil {
		if parameter["status"] == "done" {
			res = gf.Success().SetMsg("附件上传成功").SetData(parameter)
		} else {
			res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		}
	}
	return
}

// 下载插件代码到本地-安装使用
func (c *sAdmindeveloper) DownCode(ctx context.Context, req *developer.DownCodeReq) (res *gf.R) {
	if suxinOnlineServicesPending {
		return gf.Failed().SetMsg("在线服务筹备中，将迁移至 Suxin 官网：https://www.suxinwl.com")
	}
	runEnv, _ := g.Cfg("app").Get(ctx, "app.runEnv")
	if runEnv.String() == "release" {
		res = gf.Failed().SetMsg("生产环境禁止操作，请在开发环境下操作")
		return
	}
	path, _ := os.Getwd()
	install_apppath := filepath.Join(path, "/devsource/codemarket/install", req.Name)
	if _, err := os.Stat(install_apppath); err == nil {
		res = gf.Success().SetMsg("本地代码已存在直接安装").SetData(true)
		return
	}
	//1.请求社区下载插件下载地址
	result, err := g.Client().Get(ctx, req.Baseurl+"/goflycode/content/getDownUrl", gconv.MapDeep(req, "p"))
	if err != nil {
		res = gf.Failed().SetMsg("请求Suxin社区获取代码商店失败").SetData(err)
		return
	}
	defer result.Close()
	var parameter gf.Map
	if err := json.Unmarshal([]byte(result.ReadAllString()), &parameter); err != nil {
		res = gf.Failed().SetMsg("请求解析数据失败").SetData(err)
		return
	}
	if gconv.Int(parameter["code"]) != 0 || g.IsEmpty(parameter["data"]) {
		res = gf.Failed().SetMsg(gf.String(parameter["message"])).SetData(parameter)
		return
	}
	//2.下载插件代码zip
	downdir := filepath.Join(path, "/devsource/codemarket/install", req.Name+".zip")
	downstatus, downdir_zippath := clogic_developer.DownFileToDir(gf.String(parameter["data"]), downdir)
	if downstatus {
		dezipdir := filepath.Join(path, "/devsource/codemarket/install", req.Name)
		err := gcompress.UnZipFile(downdir_zippath, dezipdir, req.Name)
		if err == nil {
			os.Remove(downdir_zippath)
			res = gf.Success().SetMsg("下载代码成功").SetData(true)
		} else {
			res = gf.Failed().SetMsg("下载代码失败").SetData(err)
		}
	} else {
		res = gf.Failed().SetMsg("下载代码失败")
	}
	return
}

// 安装插件
func (c *sAdmindeveloper) InstallCode(ctx context.Context, req *developer.InstallCodeReq) (res *gf.R) {
	if regexp.MustCompile(`^runtime-[0-9a-f]{48}$`).MatchString(req.Name) {
		file := filepath.Join("storage", "plugins", "uploads", req.Name+".zip")
		if err := runtimeplugin.Default.Install(ctx, file); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		_ = os.Remove(file)
		return gf.Success().SetMsg("运行插件已安装，主服务无需重启").SetData(true)
	}
	if runtimeplugin.Default.Has(req.Name) {
		if err := runtimeplugin.Default.SetInstalled(req.Name, true); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		return gf.Success().SetMsg("运行插件已启用").SetData(true)
	}
	if plugins.Active(req.Name) {
		if err := plugins.SetInstalled(req.Name, true); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		return gf.Success().SetMsg("内置插件已启用").SetData(true)
	}
	return gf.Failed().SetMsg("请先上传与服务器平台一致的 suxin-runtime-v1 运行包")
}

// Uninstall removes execution only; never delete source, tables, files or grants.
func (c *sAdmindeveloper) UninstallCode(ctx context.Context, req *developer.UninstallCodeReq) (res *gf.R) {
	if runtimeplugin.Default.Has(req.Name) {
		if err := runtimeplugin.Default.SetInstalled(req.Name, false); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		return gf.Success().SetMsg("插件已卸载，运行包与业务数据已保留").SetData(true)
	}
	if plugins.Active(req.Name) {
		if err := plugins.SetInstalled(req.Name, false); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		return gf.Success().SetMsg("插件已卸载，业务数据已保留").SetData(true)
	}
	return gf.Failed().SetMsg("未找到已安装的插件")
}

// 安装本地插件
func (c *sAdmindeveloper) InstallLocalCode(ctx context.Context, req *developer.InstallLocalCodeReq) (res *gf.R) {
	if req.File == nil {
		return gf.Failed().SetMsg("请选择插件 ZIP")
	}
	path, err := os.Getwd()
	if err != nil {
		res = gf.Failed().SetMsg("项目路径获取失败")
		return
	}
	installPath := "/devsource/codemarket/install"
	downpathzip := filepath.Join(path, installPath)
	codeNames, err := req.File.Save(downpathzip)
	if err != nil {
		res = gf.Failed().SetMsg("上传本地插件包失败").SetData(err)
		return
	}
	zipFilePath := filepath.Join(path, installPath, codeNames)
	if runtimeplugin.IsRuntime(zipFilePath) {
		defer os.Remove(zipFilePath)
		if _, err := runtimeplugin.Inspect(zipFilePath); err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		token, err := runtimeplugin.StageUpload(zipFilePath)
		if err != nil {
			return gf.Failed().SetMsg(err.Error())
		}
		return gf.Success().SetMsg("运行包已校验，等待安装").SetData(token)
	}
	_ = os.Remove(zipFilePath)
	return gf.Failed().SetMsg("这是源码包，请在开发机编译为 suxin-runtime-v1 运行包后上传；服务器安装不会调用 Go 或 npm")
}

// 查找本地已经安装的包
func (c *sAdmindeveloper) GetInstallPack(ctx context.Context, req *developer.GetInstallPackReq) (res *gf.R) {
	if req.Catalog {
		items := make([]interface{}, 0)
		entries := map[string]string{}
		for _, item := range plugins.Catalog() {
			entries[item.Name] = item.Entry
			if !runtimeplugin.Default.Has(item.Name) {
				items = append(items, item)
			}
		}
		for _, item := range runtimeplugin.Default.Statuses() {
			if entry := entries[item["name"].(string)]; entry != "" {
				item["entry"] = entry
			}
			items = append(items, item)
		}
		return gf.Success().SetData(items)
	}
	path, err := os.Getwd()
	if err != nil {
		res = gf.Failed().SetMsg("项目路径获取失败")
		return
	}
	pathname := filepath.Join(path, "/manifest/codeinstall")
	rd, err := os.ReadDir(pathname)
	if err != nil {
		res = gf.Success().SetMsg("本地没有安装的包").SetData("")
		return
	}
	var folders = make([]string, 0)
	for _, fi := range rd {
		if fi.IsDir() {
			if plugins.Active(fi.Name()) && !plugins.Enabled(fi.Name()) {
				continue
			}
			folders = append(folders, fi.Name())
		}
	}
	res = gf.Success().SetMsg("本地已经安装的包").SetData(strings.Join(folders, ","))
	return
}
