package uploads

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/qiniu/go-sdk/v7/auth"
	"github.com/qiniu/go-sdk/v7/storage"
)

// 七牛云对象存储
type QiniuOSS struct {
	Mac     *auth.Credentials
	Ready   bool
	Config  Config
	upToken string
}

// 初始化客户端
func (m *QiniuOSS) InitClient(config Config) {
	m.Ready = false
	m.Config = config
	m.Mac = auth.New(config.KeyId, config.Secret)
	putPlicy := storage.PutPolicy{
		Scope: config.BucketName,
	}
	// 获取上传凭证
	m.upToken = putPlicy.UploadToken(m.Mac)

	m.Ready = true
}

// 上传文件
func (m *QiniuOSS) UploadFile(file *ghttp.UploadFile) (url, cover_url string, err error) {
	if !m.Ready {
		err = errors.New("not ready")
		return
	}
	// 配置参数
	cfg := storage.Config{
		Zone:          m.selectZone(), // 华南区
		UseCdnDomains: false,
		UseHTTPS:      false, // 非https
	}
	formUploader := storage.NewFormUploader(&cfg)
	ret := storage.PutRet{}        // 上传后返回的结果
	putExtra := storage.PutExtra{} // 额外参数

	fd, err := file.Open()
	if err != nil {
		return
	}
	defer fd.Close()
	//注意:文件名称必须以qiniuoss开头
	fmt.Println("1.七牛：", m.Config.DirPath)
	path_name := fmt.Sprintf("%v/%v/%v", m.Config.DirPath, time.Now().Format("20060102"), MakeFileName(file, "qiniuoss"))
	url = path_name
	err = formUploader.Put(context.Background(), &ret, m.upToken, path_name, fd, file.Size, &putExtra)
	return
}

// 下载文件
func (m *QiniuOSS) DownloadFile(fileUrl string) error {
	if !m.Ready {
		return errors.New("not ready")
	}
	deadline := time.Now().Add(time.Second * 3600).Unix() //1小时有效期
	privateAccessURL := storage.MakePrivateURL(m.Mac, m.Config.Endpoint, fileUrl, deadline)
	fmt.Println("七牛下载文件", privateAccessURL)
	//处理保存本地
	return nil
}

// 删除文件
func (m *QiniuOSS) RemoveFile(fileUrl string) error {
	if !m.Ready {
		return errors.New("not ready")
	}
	cfg := storage.Config{
		// 是否使用https域名进行资源管理
		UseHTTPS: false,
	}
	bucketManager := storage.NewBucketManager(m.Mac, &cfg)
	err := bucketManager.Delete(m.Config.BucketName, fileUrl)
	return err
}

// 移动文件，fileUrl原来文件所在位置，targerDir目标位置路径
func (m *QiniuOSS) MoveFile(fileUrl string, targerDir string) (string, error) {
	// 本地移动
	fileUrl, _ = InitFileUrl(fileUrl, m.Config)
	targerUrl := path.Join(targerDir, filepath.Base(fileUrl))
	targerUrl, _ = InitFileUrl(targerUrl, m.Config)
	MoveFile(fileUrl, targerUrl)
	// 七牛云移动
	cfg := storage.Config{
		// 是否使用https域名进行资源管理
		UseHTTPS: false,
	}
	bucketManager := storage.NewBucketManager(m.Mac, &cfg)
	// 01. 拷贝
	_, fileUrl = InitFileUrl(fileUrl, m.Config)
	_, targerUrl = InitFileUrl(targerUrl, m.Config)
	//如果目标文件存在，是否强制覆盖，如果不覆盖，默认返回614 file exists
	force := false
	err := bucketManager.Move(m.Config.BucketName, fileUrl, m.Config.DestBucketName, targerUrl, force)
	if err != nil {
		return "", err
	}
	// 02. 删除原来位置附件
	err = bucketManager.Delete(m.Config.BucketName, fileUrl)
	if err != nil {
		return "", err
	}
	return targerUrl, nil

}

// 存储区域
func (m *QiniuOSS) selectZone() *storage.Zone {
	switch m.Config.Zone {
	case 1: // 华东机房
		return &storage.ZoneHuadong
	case 2: // 华东浙江 2 区
		return &storage.ZoneHuadongZheJiang2
	case 3: // 华北机房
		return &storage.ZoneHuabei
	case 4: // 华南机房
		return &storage.ZoneHuanan
	case 5: // 北美机房
		return &storage.ZoneBeimei
	case 6: // 新加坡机房
		return &storage.ZoneXinjiapo
	default:
		return &storage.ZoneHuadong
	}
}
