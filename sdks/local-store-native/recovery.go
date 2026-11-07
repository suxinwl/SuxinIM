package localstore

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"golang.org/x/crypto/scrypt"
)

type recoveryEnvelope struct {
	Format  string `json:"format"`
	Account string `json:"account"`
	KDF     string `json:"kdf"`
	Salt    string `json:"salt"`
	Data    string `json:"data"`
}

// ExportRecovery implements the shared password-wrapped key package. The
// password never derives the ordinary working/master key; it wraps a copy only.
func ExportRecovery(master []byte, account, passphrase string) ([]byte, error) {
	if len(master) != 32 || !validField(account) || utf8.RuneCountInString(passphrase) < 12 || len(passphrase) > 4096 {
		return nil, errors.New("invalid recovery input")
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	var sealed bytes.Buffer
	if err = Encrypt(&sealed, bytes.NewReader(master), key, ObjectIdentity{Account: account, Type: "recovery", ObjectID: "master-key"}); err != nil {
		return nil, err
	}
	return json.Marshal(recoveryEnvelope{Format: Format, Account: account, KDF: "scrypt-32768-8-1", Salt: base64.StdEncoding.EncodeToString(salt), Data: base64.StdEncoding.EncodeToString(sealed.Bytes())})
}
func ImportRecovery(envelope []byte, account, passphrase string) ([]byte, error) {
	if len(envelope) > 65536 || utf8.RuneCountInString(passphrase) < 12 || len(passphrase) > 4096 {
		return nil, errors.New("invalid recovery package")
	}
	var value recoveryEnvelope
	decoder := json.NewDecoder(bytes.NewReader(envelope))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return nil, errors.New("invalid recovery package")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("invalid recovery package trailing data")
	}
	if value.Format != Format || value.Account != account || value.KDF != "scrypt-32768-8-1" {
		return nil, errors.New("recovery identity mismatch")
	}
	salt, err := base64.StdEncoding.Strict().DecodeString(value.Salt)
	if err != nil || len(salt) != 16 {
		return nil, errors.New("invalid recovery salt")
	}
	sealed, err := base64.StdEncoding.Strict().DecodeString(value.Data)
	if err != nil {
		return nil, errors.New("invalid recovery ciphertext")
	}
	key, err := scrypt.Key([]byte(passphrase), salt, 32768, 8, 1, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	var plain bytes.Buffer
	if err = Decrypt(&plain, bytes.NewReader(sealed), key, ObjectIdentity{Account: account, Type: "recovery", ObjectID: "master-key"}); err != nil {
		return nil, errors.New("recovery authentication failed")
	}
	if plain.Len() != 32 {
		clear(plain.Bytes())
		return nil, errors.New("invalid recovered master")
	}
	return plain.Bytes(), nil
}
