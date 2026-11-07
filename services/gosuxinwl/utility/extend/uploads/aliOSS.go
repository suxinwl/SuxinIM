package uploads

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
)

// 阿里云 OSS 文档
// https://help.aliyun.com/zh/oss/developer-reference/introduction-3?spm=5176.8466032.console-base_help.dexternal.67181450WWM1Nx

type AliOSS struct {
	Client *oss.Client
	Ready  bool
	Config Config
	Bucket *oss.Bucket
}

func (m *AliOSS) InitClient(config Config) {
	m.Ready = false
	m.Config = config
	client, err := oss.New(config.Endpoint, config.KeyId, config.Secret)
	if err != nil {
		return
	}
	m.Client = client
	m.Bucket, err = client.Bucket(config.BucketName)
	if err != nil {
		return
	}
	m.Ready = true
}

// 上传文件
func (m *AliOSS) UploadFile(file *ghttp.UploadFile) (url, cover_url string, err error) {
	if !m.Ready {
		err = errors.New("not ready")
		return
	}
	fd, err := file.Open()
	if err != nil {
		return
	}
	defer fd.Close()
	//注意:文件名称必须以alioss开头,MakeFileName函数就是生成以alioss开头的文件名称
	path_name := fmt.Sprintf("%v/%v/%v", m.Config.DirPath, time.Now().Format("20060102"), MakeFileName(file, "alioss"))
	url = path_name
	err = m.Bucket.PutObject(path_name, fd)
	return
}

// 删除文件（单个）
func (m *AliOSS) RemoveFile(fileUrl string) error {
	if !m.Ready {
		return errors.New("not ready")
	}
	return m.Bucket.DeleteObject(fileUrl)
}

// 下载文件
func (m *AliOSS) DownloadFile(fileUrl string) error {
	fileUrl, cloudFileUrl := InitFileUrl(fileUrl, m.Config)
	if !m.Ready {
		return errors.New("not ready")
	}
	// 检查对象是否存在
	_, err := m.Bucket.GetObjectMeta(cloudFileUrl)
	if err != nil {
		return err
	}
	// 下载文件
	return m.Bucket.GetObjectToFile(cloudFileUrl, fileUrl)
}

// 移动文件
func (m *AliOSS) MoveFile(fileUrl string, targerDir string) (string, error) {

	// 本地移动
	fileUrl, _ = InitFileUrl(fileUrl, m.Config)
	targerUrl := path.Join(targerDir, filepath.Base(fileUrl))
	targerUrl, _ = InitFileUrl(targerUrl, m.Config)
	MoveFile(fileUrl, targerUrl)

	// 阿里云移动
	// 01. 拷贝
	_, fileUrl = InitFileUrl(fileUrl, m.Config)
	_, targerUrl = InitFileUrl(targerUrl, m.Config)
	_, err := m.Bucket.CopyObject(fileUrl, targerUrl)
	if err != nil {
		return "", err
	}
	// 02. 删除
	err = m.Bucket.DeleteObject(fileUrl)
	if err != nil {
		return "", err
	}
	return targerUrl, nil
}
