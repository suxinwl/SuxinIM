package pan123

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/gconv"
)

const Prefix = "/common/storage/pan123/"
const MaxFileSize int64 = 10 * 1024 * 1024 * 1024

type Config struct {
	ClientID     string `json:"clientID" yaml:"clientID"`
	ClientSecret string `json:"clientSecret" yaml:"clientSecret"`
	ParentID     int64  `json:"parentId" yaml:"parentId"`
	URLAuth      *bool  `json:"urlAuth" yaml:"urlAuth"`
	CDNKey       string `json:"cdnKey" yaml:"cdnKey"`
	UID          int64  `json:"uid" yaml:"uid"`
}

func (p Config) URLAuthEnabled() bool { return p.URLAuth == nil || *p.URLAuth }
func Load(ctx context.Context) (Config, error) {
	var p Config
	v, e := g.Cfg("upload").Get(ctx, "pan123")
	if e != nil {
		return p, e
	}
	if !v.IsNil() {
		e = gconv.Struct(v.Map(), &p)
	}
	return p, e
}
func (p Config) Validate() error {
	if strings.TrimSpace(p.ClientID) == "" || strings.TrimSpace(p.ClientSecret) == "" {
		return errors.New("请填写 123 云盘 clientID 和 clientSecret")
	}
	if p.ParentID < 0 {
		return errors.New("目标目录 ID 不能小于零")
	}
	if p.URLAuthEnabled() && p.CDNKey == "" {
		return errors.New("开启 URL 鉴权时请填写 CDN 鉴权密钥")
	}
	return nil
}

// Merge keeps write-only credentials when an edit leaves their inputs blank.
func Merge(old, p Config) (Config, error) {
	if p.ClientSecret == "" {
		if p.ClientID != old.ClientID {
			return p, errors.New("修改clientID时必须填写对应clientSecret")
		}
		p.ClientSecret = old.ClientSecret
	}
	if p.CDNKey == "" && p.ClientID == old.ClientID {
		p.CDNKey = old.CDNKey
	}
	return p, p.Validate()
}
func newKey() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func writePrivateFile(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".token-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e != nil {
		return e
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}

// ValidateAccount deliberately uses the candidate credentials before a configuration is saved.
func ValidateAccount(ctx context.Context, p Config) (int64, error) {
	if e := p.Validate(); e != nil {
		return 0, e
	}
	d, e := clientFor(p).request(ctx, http.MethodGet, "https://open-api.123pan.com/api/v1/user/info", nil, true)
	if e != nil {
		return 0, e
	}
	uid := panNumber(d, "uid")
	if uid <= 0 {
		return 0, errors.New("123 云盘未返回有效 UID")
	}
	return uid, nil
}

var referencePattern = regexp.MustCompile(`^/common/storage/pan123/([1-9][0-9]*)/([1-9][0-9]*)/pan123[0-9a-f]{32}(\.[a-zA-Z0-9]{1,16})?$`)

func ParseReference(raw string) (path string, uid, id int64, err error) {
	u, e := url.Parse(raw)
	if e != nil {
		return "", 0, 0, errors.New("无效的云盘附件地址")
	}
	path = u.Path
	m := referencePattern.FindStringSubmatch(path)
	if m == nil {
		return "", 0, 0, errors.New("无效的云盘附件标识")
	}
	uid, e = strconv.ParseInt(m[1], 10, 64)
	if e != nil {
		return "", 0, 0, e
	}
	id, e = strconv.ParseInt(m[2], 10, 64)
	return path, uid, id, e
}

func Upload(ctx context.Context, p Config, localPath, extension string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	uid, e := ValidateAccount(ctx, p)
	if e != nil {
		return "", e
	}
	if p.UID != 0 && uid != p.UID {
		return "", errors.New("云盘账号已改变，请重新配置")
	}
	p.UID = uid
	st, e := os.Stat(localPath)
	if e != nil {
		return "", e
	}
	if st.Size() > MaxFileSize {
		return "", errors.New("123 云盘单文件最大支持 10GB")
	}
	key, e := newKey()
	if e != nil {
		return "", e
	}
	if extension != "" && !regexp.MustCompile(`^\.[a-zA-Z0-9]{1,16}$`).MatchString(extension) {
		return "", errors.New("不支持的文件扩展名")
	}
	name := "pan123" + key + extension
	c := clientFor(p)
	id, e := c.upload(ctx, localPath, time.Now().Format("20060102")+"/"+name, p.ParentID)
	if e != nil {
		return "", e
	}
	if _, e = c.direct(ctx, id); e != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if cleanupErr := trash(cleanup, c, id); cleanupErr != nil {
			return "", fmt.Errorf("%w；回收云端文件 %d 失败：%v", e, id, cleanupErr)
		}
		return "", e
	}
	return fmt.Sprintf("%s%d/%d/%s", Prefix, uid, id, name), nil
}
func URL(ctx context.Context, p Config, reference string) (string, error) {
	_, uid, id, e := ParseReference(reference)
	if e != nil {
		return "", e
	}
	if p.UID != uid {
		return "", errors.New("附件所属云盘账号与当前配置不一致")
	}
	if e = p.Validate(); e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	raw, e := clientFor(p).direct(ctx, id)
	if e != nil {
		return "", e
	}
	return profileCDNURL(raw, p, time.Now().Add(60*time.Second))
}
func trash(ctx context.Context, c *panClient, id int64) error {
	_, e := c.request(ctx, http.MethodPost, "https://open-api.123pan.com/api/v1/file/trash", g.Map{"fileIDs": []int64{id}}, true)
	return e
}
func Remove(ctx context.Context, p Config, reference string) error {
	_, uid, id, e := ParseReference(reference)
	if e != nil {
		return e
	}
	if p.UID != uid {
		return errors.New("不能删除其他云盘账号的附件")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c := clientFor(p)
	d, e := c.detail(ctx, id)
	if e != nil {
		return e
	}
	if panNumber(d, "trashed") == 0 {
		e = trash(ctx, c, id)
	}
	if e == nil {
		directLinks.Delete(fmt.Sprintf("%s:%d:%d", p.ClientID, p.UID, id))
	}
	return e
}

type TestStep struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func Test(ctx context.Context, p Config) (steps []TestStep, uid int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	step := func(name string, e error) bool {
		msg := "通过"
		if e != nil {
			msg = e.Error()
		}
		steps = append(steps, TestStep{name, e == nil, msg})
		return e == nil
	}
	uid, err = ValidateAccount(ctx, p)
	if !step("凭证与账号", err) {
		return
	}
	p.UID = uid
	c := clientFor(p)
	if p.ParentID > 0 {
		var d g.Map
		d, err = c.detail(ctx, p.ParentID)
		if err == nil && (panNumber(d, "type") != 1 || panNumber(d, "trashed") != 0) {
			err = errors.New("目标目录不存在、已删除或不是文件夹")
		}
	}
	if !step("目标目录", err) {
		return
	}
	// Small fixtures exercise the same upload/checksum/CDN path as normal attachments.
	png, _ := hex.DecodeString("89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000b49444154789c636000020000050001a5f645400000000049454e44ae426082")
	fixtures := []struct {
		ext  string
		data []byte
	}{{".png", png}, {".pdf", testPDF()}}
	for _, fixture := range fixtures {
		f, e := os.CreateTemp("", "pan123-test-*")
		if e != nil {
			err = e
			step("临时文件", e)
			return
		}
		_, e = f.Write(fixture.data)
		ce := f.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			os.Remove(f.Name())
			err = e
			step("临时文件", e)
			return
		}
		key, e := newKey()
		if e != nil {
			os.Remove(f.Name())
			err = e
			return
		}
		id, e := c.upload(ctx, f.Name(), "_upload_connection_test/pan123"+key+fixture.ext, p.ParentID)
		os.Remove(f.Name())
		if !step(fixture.ext+" 上传与校验", e) {
			err = e
			return
		}
		func() {
			defer func() {
				cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
				defer done()
				ce := trash(cleanup, c, id)
				step(fixture.ext+" 测试文件回收", ce)
				if err == nil {
					err = ce
				}
			}()
			raw, e := c.direct(ctx, id)
			if e == nil {
				raw, e = profileCDNURL(raw, p, time.Now().Add(60*time.Second))
			}
			if e == nil {
				e = checkDownload(ctx, c, raw, fixture.data, false)
			}
			if !step(fixture.ext+" 直链访问", e) {
				err = e
				return
			}
			if p.URLAuthEnabled() {
				expired, e := signPanURL(raw, p.UID, p.CDNKey, time.Now().Add(-24*time.Hour))
				if e == nil {
					e = checkDownload(ctx, c, expired, nil, true)
				}
				step(fixture.ext+" 过期签名拒绝", e)
				err = e
			}
		}()
		if err != nil {
			return
		}
	}
	return
}
func testPDF() []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 72] /Resources << >> >>"}
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	return b.Bytes()
}

func checkDownload(ctx context.Context, c *panClient, address string, want []byte, denied bool) error {
	req, e := http.NewRequestWithContext(ctx, "GET", address, nil)
	if e != nil {
		return e
	}
	res, e := panCDNClient(c.http).Do(req)
	if e != nil {
		return errors.New("直链访问失败")
	}
	defer res.Body.Close()
	if denied {
		if res.StatusCode == 401 || res.StatusCode == 403 {
			return nil
		}
		return errors.New("过期签名仍可访问，请检查 123 控制台 URL 鉴权设置")
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("直链返回 HTTP %d，请检查目录直链和鉴权设置", res.StatusCode)
	}
	got, e := io.ReadAll(io.LimitReader(res.Body, int64(len(want)+1)))
	if e != nil {
		return e
	}
	if !bytes.Equal(got, want) {
		return errors.New("直链下载内容校验失败")
	}
	return nil
}
