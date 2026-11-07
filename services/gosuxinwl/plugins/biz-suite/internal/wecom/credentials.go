package wecom

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

// CredentialUpdate is write-only. Never include it in settings/status responses.
// Blank values preserve saved credentials; clearing is an explicit operation.
type CredentialUpdate struct {
	ContactSecret string `json:"contact_secret"`
	AppSecret     string `json:"app_secret"`
	ClearContact  bool   `json:"clear_contact_secret"`
	ClearApp      bool   `json:"clear_app_secret"`
}

func (u *CredentialUpdate) validate() error {
	u.ContactSecret, u.AppSecret = strings.TrimSpace(u.ContactSecret), strings.TrimSpace(u.AppSecret)
	for _, item := range []struct {
		value string
		clear bool
	}{{u.ContactSecret, u.ClearContact}, {u.AppSecret, u.ClearApp}} {
		if item.clear && item.value != "" {
			return errors.New("不能同时填写和清除同一套 Secret")
		}
		if len(item.value) > 512 || strings.ContainsAny(item.value, "*•●") || strings.IndexFunc(item.value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
			return errors.New("Secret 格式无效，请填写原始密钥，不能使用掩码或空白字符")
		}
	}
	return nil
}

// Master keys are separate from the database and release package. Windows keys
// are additionally protected by DPAPI for the account running the host service.
func credentialKey(create bool) ([]byte, error) {
	if value := os.Getenv("WECOM_CREDENTIAL_KEY"); value != "" {
		key, err := base64.StdEncoding.DecodeString(value)
		if err != nil || len(key) != 32 {
			return nil, errors.New("WECOM_CREDENTIAL_KEY 必须是 Base64 编码的 32 字节密钥")
		}
		return key, nil
	}
	dir := os.Getenv("SUXIN_PLUGIN_DATA")
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("未配置凭据加密密钥或插件私有数据目录")
	}
	path := filepath.Join(dir, "wecom-credentials.key")
	raw, err := os.ReadFile(path)
	if err == nil {
		return decodeCredentialKey(raw)
	}
	if !os.IsNotExist(err) {
		return nil, errors.New("无法读取凭据加密密钥")
	}
	if !create {
		return nil, errors.New("凭据加密密钥缺失，请恢复原密钥后重启服务")
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, errors.New("无法生成凭据加密密钥")
	}
	protected, err := protectCredentialKey(key)
	if err != nil {
		return nil, errors.New("无法保护凭据加密密钥")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("无法创建插件私有数据目录")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if os.IsExist(err) {
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, errors.New("无法读取凭据加密密钥")
		}
		return decodeCredentialKey(raw)
	}
	if err != nil {
		return nil, errors.New("无法保存凭据加密密钥")
	}
	_, writeErr := file.Write(protected)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return nil, errors.New("凭据加密密钥写入失败，请检查私有数据目录")
	}
	return key, nil
}

func decodeCredentialKey(raw []byte) ([]byte, error) {
	key, err := unprotectCredentialKey(raw)
	if err != nil || len(key) != 32 {
		return nil, errors.New("凭据加密密钥无效或服务账号已改变，请恢复原密钥及运行账号")
	}
	return key, nil
}

func credentialCipher(create bool) (cipher.AEAD, error) {
	key, err := credentialKey(create)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.New("无法初始化凭据加密")
	}
	return cipher.NewGCM(block)
}

func encryptCredential(scope, secret string) (string, error) {
	aead, err := credentialCipher(true)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", errors.New("无法生成凭据随机数")
	}
	sealed := aead.Seal(nonce, nonce, []byte(secret), []byte("biz-suite/wecom/"+scope))
	return "v1:" + base64.StdEncoding.EncodeToString(sealed), nil
}

func decryptCredential(scope, value string) (string, error) {
	if !strings.HasPrefix(value, "v1:") {
		return "", errors.New("不支持的凭据加密版本")
	}
	aead, err := credentialCipher(false)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "v1:"))
	if err != nil || len(raw) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("凭据密文无效")
	}
	plain, err := aead.Open(nil, raw[:aead.NonceSize()], raw[aead.NonceSize():], []byte("biz-suite/wecom/"+scope))
	if err != nil {
		return "", errors.New("凭据解密失败，请检查原加密密钥")
	}
	if len(plain) == 0 {
		return "", errors.New("已保存的凭据为空")
	}
	return string(plain), nil
}

// Caller holds runMu during startup/configuration. A corrupt ciphertext fails
// initialization instead of silently using an unrelated environment credential.
func (c *Client) loadCredentials(ctx context.Context) error {
	rows, err := core.Model("wecom_credentials").Ctx(ctx).All()
	if err != nil {
		return dbError("credentials load")
	}
	contact, app := c.envContactSecret, c.envAppSecret
	savedContact, savedApp := false, false
	for _, row := range rows {
		scope := row["scope"].String()
		if scope != "contact" && scope != "app" {
			return errors.New("未知的企业微信凭据类型")
		}
		secret, err := decryptCredential(scope, row["encrypted_secret"].String())
		if err != nil {
			return err
		}
		if scope == "contact" {
			contact, savedContact = secret, true
		} else {
			app, savedApp = secret, true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ContactSecret, c.AppSecret, c.Secret = contact, app, c.envLegacySecret
	c.savedContactSecret, c.savedAppSecret = savedContact, savedApp
	c.tokens = nil
	return nil
}
