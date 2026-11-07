package uploads

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/utility/extend/pan123"
)

type Pan123 struct {
	config pan123.Config
	ctx    context.Context
	err    error
}

func (p *Pan123) InitClient(Config) {}
func (p *Pan123) UploadFile(file *ghttp.UploadFile) (string, string, error) {
	if p.err != nil {
		return "", "", p.err
	}
	if file == nil {
		return "", "", errors.New("未上传文件")
	}
	if file.Size > pan123.MaxFileSize {
		return "", "", errors.New("123 云盘单文件最大支持 10GB")
	}
	in, e := file.Open()
	if e != nil {
		return "", "", e
	}
	defer in.Close()
	f, e := os.CreateTemp("", "pan123-upload-*")
	if e != nil {
		return "", "", e
	}
	defer os.Remove(f.Name())
	_, e = io.Copy(f, io.LimitReader(in, pan123.MaxFileSize+1))
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return "", "", e
	}
	u, e := pan123.Upload(p.ctx, p.config, f.Name(), strings.ToLower(filepath.Ext(file.Filename)))
	return u, "", e
}
func (p *Pan123) RemoveFile(u string) error {
	if p.err != nil {
		return p.err
	}
	return pan123.Remove(p.ctx, p.config, u)
}
func (p *Pan123) DownloadFile(string) error {
	return errors.New("请通过附件访问地址下载123云盘文件")
}
func (p *Pan123) MoveFile(string, string) (string, error) {
	return "", errors.New("123云盘附件目录由数据中心管理，不直接移动云端文件")
}

// WithContext preserves the original provider interface while allowing cancellation for cloud IO.
func WithContext(ctx context.Context, upType string) StaticCloud {
	if upType == "pan123" {
		p, e := pan123.Load(ctx)
		return &Pan123{config: p, ctx: ctx, err: e}
	}
	return New(upType)
}
