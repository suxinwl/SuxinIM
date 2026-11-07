//go:build windows

package wecom

import (
	"bytes"
	"errors"
	"unsafe"

	winapi "golang.org/x/sys/windows"
)

func protectCredentialKey(key []byte) ([]byte, error) {
	input := winapi.DataBlob{Size: uint32(len(key)), Data: &key[0]}
	var output winapi.DataBlob
	if err := winapi.CryptProtectData(&input, nil, nil, 0, nil, winapi.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, err
	}
	defer winapi.LocalFree(winapi.Handle(unsafe.Pointer(output.Data)))
	return append([]byte("WDP1"), unsafe.Slice(output.Data, int(output.Size))...), nil
}

func unprotectCredentialKey(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte("WDP1")) || len(raw) < 5 {
		return nil, errors.New("invalid protected key")
	}
	input := winapi.DataBlob{Size: uint32(len(raw) - 4), Data: &raw[4]}
	var output winapi.DataBlob
	if err := winapi.CryptUnprotectData(&input, nil, nil, 0, nil, winapi.CRYPTPROTECT_UI_FORBIDDEN, &output); err != nil {
		return nil, err
	}
	defer winapi.LocalFree(winapi.Handle(unsafe.Pointer(output.Data)))
	return append([]byte(nil), unsafe.Slice(output.Data, int(output.Size))...), nil
}
