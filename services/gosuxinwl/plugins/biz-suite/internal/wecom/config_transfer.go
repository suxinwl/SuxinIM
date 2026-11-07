package wecom

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type connectionBackup struct {
	Version  int      `json:"version"`
	Settings Settings `json:"settings"`
	Contact  string   `json:"contact"`
	App      string   `json:"app"`
}

func transferCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("配置迁移密钥必须为32字节")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// ExportConnection decrypts source credentials only in memory, under the old
// service account, then wraps the entire configuration with a portable key.
func ExportConnection(ctx context.Context, key []byte) ([]byte, error) {
	client := NewFromEnv()
	if err := client.LoadSettings(ctx); err != nil {
		return nil, err
	}
	settings := client.Settings()
	settings.CorpID, settings.AgentID = client.CorpID, client.AgentID
	if settings.CorpID == "" || client.secret("contact") == "" || client.secret("app") == "" {
		return nil, errors.New("企业微信连接配置不完整，拒绝导出")
	}
	plain, err := json.Marshal(connectionBackup{1, settings, client.secret("contact"), client.secret("app")})
	if err != nil {
		return nil, err
	}
	aead, err := transferCipher(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, plain, []byte("gosuxin-wecom-connection-v1")), nil
}

// RestoreConnection accepts only empty settings/credential tables. It re-seals
// secrets with the target deployment key; no users, history or cursors transfer.
func RestoreConnection(ctx context.Context, key, sealed []byte) error {
	aead, err := transferCipher(key)
	if err != nil {
		return err
	}
	if len(sealed) < aead.NonceSize()+aead.Overhead() {
		return errors.New("配置备份无效")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte("gosuxin-wecom-connection-v1"))
	if err != nil {
		return errors.New("配置备份解密失败")
	}
	var value connectionBackup
	if json.Unmarshal(plain, &value) != nil || value.Version != 1 {
		return errors.New("配置备份版本无效")
	}
	for _, table := range []string{"wecom_settings", "wecom_credentials"} {
		n, e := core.Model(table).Ctx(ctx).Count()
		if e != nil {
			return e
		}
		if n != 0 {
			return errors.New("仅允许向空配置表恢复连接配置")
		}
	}
	client := NewFromEnv()
	if err = client.LoadSettings(ctx); err != nil {
		return err
	}
	return client.SaveConfiguration(ctx, value.Settings, CredentialUpdate{ContactSecret: value.Contact, AppSecret: value.App})
}
