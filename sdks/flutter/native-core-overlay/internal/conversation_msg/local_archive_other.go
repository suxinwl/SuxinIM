//go:build !android && !ios

package conversation_msg

import (
	"github.com/openimsdk/openim-sdk-core/v3/pkg/utils"
	"github.com/openimsdk/openim-sdk-core/v3/sdk_struct"
	"io"
	"os"
)

const nativeEncryptedSources = false

func markNativeArchiveReady(_ string, _ bool)                     {}
func nativeMessageVisible(_ string, _ *sdk_struct.MsgStruct) bool { return true }
func (c *Conversation) prepareNativeSource(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	_, err := utils.CopyFile(path, utils.FileTmpPath(path, c.DataDir))
	return path, err
}
func openNativeSource(path string) (io.ReadCloser, os.FileInfo, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	return file, info, nil
}
func nativeSourceStat(path string) (os.FileInfo, error)                              { return os.Stat(path) }
func (c *Conversation) bindNativeSources(_ *sdk_struct.MsgStruct, _ ...string) error { return nil }
func (c *Conversation) releaseNativeSources(_ string)                                {}
