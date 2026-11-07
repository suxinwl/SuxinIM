package localstore

import (
	"bytes"
	"io"
	"testing"
)

func TestAuthenticatedSeekAndRevocation(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	id := ObjectIdentity{"account", "object", "outgoing"}
	body := bytes.Repeat([]byte("abcdef"), ChunkSize/2+10)
	var sealed bytes.Buffer
	if err := Encrypt(&sealed, bytes.NewReader(body), key, id); err != nil {
		t.Fatal(err)
	}
	allowed := true
	check := func() error {
		if !allowed {
			return ErrInvalidObject
		}
		return nil
	}
	reader, err := OpenAuthenticatedReader(bytes.NewReader(sealed.Bytes()), int64(sealed.Len()), key, id, check)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, offset := range []int64{0, ChunkSize - 2, ChunkSize + 3, int64(len(body) - 11)} {
		if _, err = reader.Seek(offset, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, 11)
		if _, err = io.ReadFull(reader, data); err != nil || !bytes.Equal(data, body[offset:offset+11]) {
			t.Fatal("seek mismatch", err)
		}
	}
	if len(reader.cached) > ChunkSize {
		t.Fatal("unbounded plaintext cache")
	}
	allowed = false
	data := make([]byte, 10)
	if n, err := reader.Read(data); n != 0 || err == nil {
		t.Fatal("revoked reader returned plaintext")
	}
}

func TestAuthenticatedReaderChecksWholeEOFBeforeAnyOutput(t *testing.T) {
	key := bytes.Repeat([]byte{3}, 32)
	id := ObjectIdentity{"account", "object", "outgoing"}
	var sealed bytes.Buffer
	_ = Encrypt(&sealed, bytes.NewReader(bytes.Repeat([]byte{'x'}, ChunkSize+1)), key, id)
	for _, mutate := range []func([]byte) []byte{func(data []byte) []byte { return data[:len(data)-1] }, func(data []byte) []byte { data[len(data)-1] ^= 1; return data }, func(data []byte) []byte { return append(data, 0) }} {
		data := mutate(bytes.Clone(sealed.Bytes()))
		if _, err := OpenAuthenticatedReader(bytes.NewReader(data), int64(len(data)), key, id, func() error { return nil }); err == nil {
			t.Fatal("invalid EOF accepted")
		}
	}
}
