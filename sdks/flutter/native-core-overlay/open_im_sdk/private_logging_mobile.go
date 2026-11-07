//go:build android || ios

package open_im_sdk

import "github.com/openimsdk/openim-sdk-core/v3/sdk_struct"

func configureNativePrivateLogging(config *sdk_struct.IMConfig) {
	config.LogFilePath = ""
	config.IsLogStandardOutput = false
}
