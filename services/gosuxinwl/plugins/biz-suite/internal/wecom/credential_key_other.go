//go:build !windows

package wecom

import (
	"bytes"
	"errors"
)

// Non-Windows deployments protect the private directory/key with POSIX modes;
// WECOM_CREDENTIAL_KEY may instead be supplied by the deployment secret store.
func protectCredentialKey(key []byte) ([]byte, error) { return append([]byte("POS1"), key...), nil }
func unprotectCredentialKey(raw []byte) ([]byte, error) {
	if !bytes.HasPrefix(raw, []byte("POS1")) || len(raw) != 36 {
		return nil, errors.New("invalid protected key")
	}
	return append([]byte(nil), raw[4:]...), nil
}
