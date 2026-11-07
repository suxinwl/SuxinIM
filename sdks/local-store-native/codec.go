// Package localstore implements the shared PUTY encrypted object format.
// Keys are provided by the native OS-key broker after an online authorization.
// The package never writes keys, plaintext, or credentials to logs or paths.
package localstore

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

const Format = "PUTY-LOCAL-STORE-1"
const ChunkSize = 1048576
const maxHeader = 16384

var magic = []byte("PUTYLS1\n")
var ErrInvalidObject = errors.New("invalid encrypted local object")

// Header field order is the lexical JSON key order required by the format.
type Header struct {
	Account     string `json:"account"`
	ChunkSize   int    `json:"chunkSize"`
	Format      string `json:"format"`
	NoncePrefix string `json:"noncePrefix"`
	ObjectID    string `json:"objectID"`
	Type        string `json:"type"`
}

type ObjectIdentity struct{ Account, ObjectID, Type string }

func validField(s string) bool {
	return s != "" && len(s) <= 512 && utf8.ValidString(s) && !strings.ContainsRune(s, '\x00')
}
func validateIdentity(id ObjectIdentity) bool {
	return validField(id.Account) && validField(id.ObjectID) && validField(id.Type)
}

func objectCipher(master []byte, id ObjectIdentity) (cipher.AEAD, error) {
	if len(master) != 32 || !validateIdentity(id) {
		return nil, ErrInvalidObject
	}
	key, err := hkdf.Key(sha256.New, master, []byte(Format), id.Account+"\x00"+id.Type+"\x00"+id.ObjectID, 32)
	if err != nil {
		return nil, ErrInvalidObject
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrInvalidObject
	}
	return cipher.NewGCM(block)
}

func frameData(header []byte, prefix []byte, index, length uint32) ([]byte, []byte) {
	nonce := make([]byte, 12)
	copy(nonce, prefix)
	binary.BigEndian.PutUint32(nonce[8:], index)
	aad := make([]byte, len(header)+8)
	copy(aad, header)
	binary.BigEndian.PutUint32(aad[len(header):], index)
	binary.BigEndian.PutUint32(aad[len(header)+4:], length)
	return nonce, aad
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) != 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// Encrypt emits independently authenticated chunks and an authenticated EOF.
// Use EncryptWithRandom only with a cryptographically secure random source.
func Encrypt(w io.Writer, r io.Reader, master []byte, id ObjectIdentity) error {
	return EncryptWithRandom(w, r, master, id, rand.Reader)
}

func EncryptWithRandom(w io.Writer, r io.Reader, master []byte, id ObjectIdentity, random io.Reader) error {
	aead, err := objectCipher(master, id)
	if err != nil {
		return err
	}
	prefix := make([]byte, 8)
	if _, err := io.ReadFull(random, prefix); err != nil {
		return err
	}
	header, err := json.Marshal(Header{id.Account, ChunkSize, Format, base64.StdEncoding.EncodeToString(prefix), id.ObjectID, id.Type})
	if err != nil {
		return ErrInvalidObject
	}
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(header)))
	for _, data := range [][]byte{magic, size[:], header} {
		if err := writeAll(w, data); err != nil {
			return err
		}
	}
	buffer := make([]byte, ChunkSize)
	defer clear(buffer)
	for index := uint32(0); ; index++ {
		n, readErr := io.ReadFull(r, buffer)
		if readErr != nil && readErr != io.EOF && readErr != io.ErrUnexpectedEOF {
			return readErr
		}
		if n != 0 {
			nonce, aad := frameData(header, prefix, index, uint32(n))
			sealed := aead.Seal(nil, nonce, buffer[:n], aad)
			binary.BigEndian.PutUint32(size[:], uint32(n))
			if err := writeAll(w, size[:]); err != nil {
				return err
			}
			if err := writeAll(w, sealed); err != nil {
				return err
			}
			if index == math.MaxUint32 {
				return ErrInvalidObject
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			if n != 0 {
				index++
			}
			nonce, aad := frameData(header, prefix, index, 0)
			clear(size[:])
			if err := writeAll(w, size[:]); err != nil {
				return err
			}
			return writeAll(w, aead.Seal(nil, nonce, nil, aad))
		}
	}
}

// Decrypt binds the entire object to its expected account, type and object ID.
// The caller must keep output private until this function authenticates EOF.
func Decrypt(w io.Writer, r io.Reader, master []byte, expected ObjectIdentity) error {
	var start [12]byte
	if _, err := io.ReadFull(r, start[:]); err != nil || !bytes.Equal(start[:8], magic) {
		return ErrInvalidObject
	}
	length := binary.BigEndian.Uint32(start[8:])
	if length == 0 || length > maxHeader {
		return ErrInvalidObject
	}
	header := make([]byte, length)
	if _, err := io.ReadFull(r, header); err != nil {
		return ErrInvalidObject
	}
	var meta Header
	decoder := json.NewDecoder(bytes.NewReader(header))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		return ErrInvalidObject
	}
	canonical, _ := json.Marshal(meta)
	if !bytes.Equal(header, canonical) || meta.ChunkSize != ChunkSize || meta.Format != Format || meta.Account != expected.Account || meta.ObjectID != expected.ObjectID || meta.Type != expected.Type {
		return ErrInvalidObject
	}
	prefix, err := base64.StdEncoding.Strict().DecodeString(meta.NoncePrefix)
	if err != nil || len(prefix) != 8 {
		return ErrInvalidObject
	}
	aead, err := objectCipher(master, expected)
	if err != nil {
		return err
	}
	var size [4]byte
	for index := uint32(0); ; index++ {
		if _, err := io.ReadFull(r, size[:]); err != nil {
			return ErrInvalidObject
		}
		length := binary.BigEndian.Uint32(size[:])
		if length > ChunkSize {
			return ErrInvalidObject
		}
		sealed := make([]byte, int(length)+aead.Overhead())
		if _, err := io.ReadFull(r, sealed); err != nil {
			return ErrInvalidObject
		}
		nonce, aad := frameData(header, prefix, index, length)
		plain, err := aead.Open(nil, nonce, sealed, aad)
		if err != nil {
			return ErrInvalidObject
		}
		if length == 0 {
			var extra [1]byte
			if n, err := r.Read(extra[:]); n != 0 || err != io.EOF {
				return ErrInvalidObject
			}
			return nil
		}
		err = writeAll(w, plain)
		clear(plain)
		if err != nil {
			return err
		}
		if index == math.MaxUint32 {
			return ErrInvalidObject
		}
	}
}
