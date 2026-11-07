//go:build !android && !ios

package open_im_sdk

import "github.com/openimsdk/openim-sdk-core/v3/sdk_struct"

func configureNativePrivateLogging(_ *sdk_struct.IMConfig) {}

func nativeLoginFence() func() { return func() {} }
