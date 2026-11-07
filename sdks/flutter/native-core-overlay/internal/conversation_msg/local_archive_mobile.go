//go:build android || ios

package conversation_msg

import (
	"encoding/json"
	"github.com/openimsdk/openim-sdk-core/v3/pkg/constant"
	"github.com/openimsdk/openim-sdk-core/v3/pkg/utils"
	"github.com/openimsdk/openim-sdk-core/v3/sdk_struct"
	"io"
	"os"
	nativestore "puty.local/nativestore"
)

const nativeEncryptedSources = true

func (c *Conversation) prepareNativeSource(path string) (string, error) {
	store, err := nativestore.Lookup(c.loginUserID)
	if err != nil {
		return "", err
	}
	return store.PrepareOutgoing(path)
}
func openNativeSource(path string) (io.ReadCloser, os.FileInfo, error) {
	reader, err := nativestore.OpenOutgoing(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := reader.Stat()
	if err != nil {
		_ = reader.Close()
		return nil, nil, err
	}
	return reader, info, nil
}
func nativeSourceStat(path string) (os.FileInfo, error) {
	reader, info, err := openNativeSource(path)
	if reader != nil {
		_ = reader.Close()
	}
	return info, err
}
func (c *Conversation) bindNativeSources(message *sdk_struct.MsgStruct, paths ...string) error {
	store, err := nativestore.Lookup(c.loginUserID)
	if err != nil {
		return err
	}
	if err = store.BindOutgoing(message.ClientMsgID, paths...); err != nil {
		return err
	}
	if message.ContentType != constant.Picture && message.ContentType != constant.Sound && message.ContentType != constant.Video {
		return nil
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		name, _, err := store.SourceInfo(path)
		if err != nil {
			return err
		}
		extra := map[string]json.RawMessage{}
		if message.Ex != "" {
			if json.Unmarshal([]byte(message.Ex), &extra) != nil || extra == nil {
				return nil
			}
		}
		encoded, _ := json.Marshal(name)
		extra["putySourceName"] = encoded
		merged, _ := json.Marshal(extra)
		message.Ex = string(merged)
		break
	}
	return nil
}
func (c *Conversation) releaseNativeSources(message string) {
	if store, err := nativestore.Lookup(c.loginUserID); err == nil {
		_ = store.ReleaseOutgoing(message)
	}
}

func markNativeArchiveReady(account string, ready bool) {
	if store, err := nativestore.Lookup(account); err == nil {
		store.MarkViewsReady(ready)
		if ready {
			_ = store.FinalizeRestoredPrevious()
		}
	}
}
func nativeMessageVisible(account string, message *sdk_struct.MsgStruct) bool {
	store, err := nativestore.Lookup(account)
	return err == nil && !store.Suppressed(utils.GetConversationIDByMsg(message), message.ClientMsgID, message.SendTime)
}
