package gf

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/container/gvar"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/i18n/gi18n"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/os/gctx"
	"github.com/suxinwl/GoSuxin/framework/os/gfile"
	"github.com/spf13/viper"
)

var (
	ctx  = gctx.New()
	i18n = gi18n.New()
)

// I18N国际化统一调用，默认英文
func I18n(r *ghttp.Request, key string) string {
	locale := r.Header.Get("locale")
	if g.IsEmpty(locale) {
		locale = "en-US"
	}
	i18n.SetLanguage(locale)
	return i18n.Translate(r.Context(), key)
}

// 获取附件访问的完整地址，传入路径返回完整
func GetFullUrl(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	if strings.HasPrefix(strings.TrimPrefix(url, "/"), "resource/static/brand/") || strings.HasPrefix(url, "/common/storage/pan123/") {
		return GetLocalFullUrl(url)
	}
	if !strings.HasPrefix(url, "/") {
		url = "/" + url
	}
	//处理地址
	filseName := gfile.Name(url)
	if strings.HasPrefix(filseName, "local") { //本地存储
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "local.LBaseUrl")
		return BaseUrl.String() + url
	} else if strings.HasPrefix(filseName, "alioss") { //阿里云
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "alioss.ABaseUrl")
		return BaseUrl.String() + url
	} else if strings.HasPrefix(filseName, "tencentcos") { //腾讯云
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "tencentcos.TBaseUrl")
		return BaseUrl.String() + url
	} else if strings.HasPrefix(filseName, "qiniuoss") { //七牛云
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "qiniuoss.QBaseUrl")
		return BaseUrl.String() + url
	} else { //默认返回设置上传方式的地址
		return GetRootUrl() + url
	}
}

// 获取本地附件访问的完整地址，传入路径返回完整
func GetLocalFullUrl(url string) string {
	if url == "" {
		return ""
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	if !strings.HasPrefix(url, "/") {
		url = "/" + url
	}
	BaseUrl, _ := g.Cfg("upload").Get(ctx, "local.LBaseUrl")
	return BaseUrl.String() + url
}

// 获取当前设置上传方式的附件访问域名，如当前是本地(local)则返回本地访问地址
func GetRootUrl() string {
	UpType, _ := g.Cfg("upload").Get(ctx, "Type")
	switch UpType.String() {
	case "alioss":
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "alioss.ABaseUrl")
		return BaseUrl.String()
	case "tencentcos":
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "tencentcos.TBaseUrl")
		return BaseUrl.String()
	case "qiniuoss":
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "qiniuoss.QBaseUrl")
		return BaseUrl.String()
	default:
		BaseUrl, _ := g.Cfg("upload").Get(ctx, "local.LBaseUrl")
		return BaseUrl.String()
	}
}

// 获取所有上传方式附件访问的地址域名
func GetAllRootUrl() g.Map {
	LBaseUrl, _ := g.Cfg("upload").Get(ctx, "local.LBaseUrl")
	ABaseUrl, _ := g.Cfg("upload").Get(ctx, "alioss.ABaseUrl")
	TBaseUrl, _ := g.Cfg("upload").Get(ctx, "tencentcos.TBaseUrl")
	QBaseUrl, _ := g.Cfg("upload").Get(ctx, "qiniuoss.QBaseUrl")
	return g.Map{
		"pan123":     LBaseUrl,
		"local":      LBaseUrl,
		"alioss":     ABaseUrl,
		"tencentcos": TBaseUrl,
		"qiniuoss":   QBaseUrl,
	}
}

// 删除单文件本地附件
func DelOneFile(file_path string) error {
	path, _ := os.Getwd()
	deldir := filepath.Join(path, file_path)
	if _, err := os.Stat(deldir); err != nil && os.IsNotExist(err) { //文件不存在直接返回
		return nil
	}
	return os.Remove(deldir)
}

// 删除本地附件
func Del_file(file_list []*gvar.Var) {
	path, _ := os.Getwd()
	for _, val := range file_list {
		deldir := filepath.Join(path, val.String())
		os.Remove(deldir)
	}
}

// 获取文文件夹下的文件及文件返回数组
func GetAllFileArray(pathname string) (map[string]interface{}, error) {
	var folders = make([]string, 0)
	var files = make([]string, 0)
	rd, err := os.ReadDir(pathname)
	if err != nil {
		return map[string]interface{}{"folders": folders, "files": files}, err
	}
	for _, fi := range rd {
		if fi.IsDir() {
			folders = append(folders, fi.Name())
		} else {
			files = append(files, fi.Name())
		}
	}
	return map[string]interface{}{"folders": folders, "files": files}, nil
}

// 逐行读取文件
// filePath文件路径
func ReaderFileByline(filePath string) []interface{} {
	f, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	buf := bufio.NewReader(f)
	var list []interface{}
	for {
		line, _, err := buf.ReadLine()
		if err == io.EOF {
			break
		}
		list = append(list, string(line))
	}
	return list
}

// 读取Yaml配置文件， struct结构
func GetYmlConfigData(path, name string) (interface{}, error) {
	var config interface{}
	vip := viper.New()
	vip.AddConfigPath(path)   //设置读取的文件路径
	vip.SetConfigName(name)   //设置读取的文件名
	vip.SetConfigType("yaml") //设置文件的类型
	//尝试进行配置读取
	if err := vip.ReadInConfig(); err != nil {
		return nil, err
	}
	verr := vip.Unmarshal(&config)
	if verr != nil {
		return nil, verr
	}
	return config, nil
}
