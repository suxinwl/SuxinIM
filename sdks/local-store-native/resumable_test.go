package localstore

import (
	"bytes"
	"testing"
)

func TestResumeAuthenticatedPrefixFreshNonceAndEOF(t *testing.T) {
	key := bytes.Repeat([]byte{0x31}, 32)
	id := ObjectIdentity{Account: "synthetic", Type: "attachment", ObjectID: "resume"}
	payload := bytes.Repeat([]byte{0x62}, 2*ChunkSize+123)
	var pending bytes.Buffer
	first, err := NewChunkWriter(&pending, key, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.Write(payload[:ChunkSize+100]); err != nil {
		t.Fatal(err)
	}
	first.Abort()
	if err = Decrypt(&bytes.Buffer{}, bytes.NewReader(pending.Bytes()), key, id); err == nil {
		t.Fatal("pending file passed ordinary authenticated read")
	}
	var continued bytes.Buffer
	next, err := NewChunkWriter(&continued, key, id)
	if err != nil {
		t.Fatal(err)
	}
	offset, complete, err := RewrapPending(bytes.NewReader(pending.Bytes()), next, key, id)
	if err != nil || complete || offset != ChunkSize {
		t.Fatal("wrong authenticated resume offset", offset, err)
	}
	if bytes.Equal(pending.Bytes()[:200], continued.Bytes()[:200]) {
		t.Fatal("resume reused old nonce header")
	}
	if _, err = next.Write(payload[offset:]); err != nil {
		t.Fatal(err)
	}
	if err = next.Finish(); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err = Decrypt(&output, bytes.NewReader(continued.Bytes()), key, id); err != nil || !bytes.Equal(output.Bytes(), payload) {
		t.Fatal("resumed bytes differ", err)
	}
}
func TestResumeRejectsTamperedAuthenticatedFrame(t *testing.T) {
	key := bytes.Repeat([]byte{0x33}, 32)
	id := ObjectIdentity{Account: "synthetic", Type: "attachment", ObjectID: "tamper"}
	var pending bytes.Buffer
	first, _ := NewChunkWriter(&pending, key, id)
	_, _ = first.Write(bytes.Repeat([]byte{0x11}, ChunkSize))
	first.Abort()
	body := pending.Bytes()
	body[len(body)-1] ^= 1
	var output bytes.Buffer
	next, _ := NewChunkWriter(&output, key, id)
	if _, _, err := RewrapPending(bytes.NewReader(body), next, key, id); err == nil {
		t.Fatal("corrupt completed frame accepted")
	}
	next.Abort()
}
func TestResumeTruncatedTailNeverReusesPrefix(t *testing.T) {
	key := bytes.Repeat([]byte{0x44}, 32)
	id := ObjectIdentity{Account: "synthetic", Type: "attachment", ObjectID: "tail"}
	var complete bytes.Buffer
	_ = Encrypt(&complete, bytes.NewReader(bytes.Repeat([]byte{0x17}, 2*ChunkSize)), key, id)
	body := complete.Bytes()
	var output bytes.Buffer
	next, _ := NewChunkWriter(&output, key, id)
	offset, finished, err := RewrapPending(bytes.NewReader(body[:len(body)-100]), next, key, id)
	if err != nil || finished || offset != ChunkSize {
		t.Fatal("partial physical frame not safely retried", offset, err)
	}
	next.Abort()
}
