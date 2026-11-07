package nativestore

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"
)

func TestOutgoingCipherSeekReferencesAndExpiredReader(t *testing.T) {
	s, _ := fixture(t)
	body := bytes.Repeat([]byte("OUTGOING_SYNTHETIC"), 90000)
	path, err := s.StageOutgoing(bytes.NewReader(body), "职员名单.zip")
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(path)
	if err != nil || bytes.Contains(disk, []byte("OUTGOING_SYNTHETIC")) {
		t.Fatal("plaintext outgoing source")
	}
	reader, err := OpenOutgoing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.Size() != int64(len(body)) {
		t.Fatal("cipher size used as source size")
	}
	_, _ = reader.Seek(1000001, io.SeekStart)
	part := make([]byte, 321)
	if _, err = io.ReadFull(reader, part); err != nil || !bytes.Equal(part, body[1000001:1000322]) {
		t.Fatal("encrypted source seek mismatch", err)
	}
	if err = s.BindOutgoing("pending-a", path); err != nil {
		t.Fatal(err)
	}
	if err = s.BindOutgoing("pending-b", path); err != nil {
		t.Fatal(err)
	}
	if err = s.ReleaseOutgoing("pending-a"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal("cross-reference source prematurely deleted")
	}
	if err = s.ReleaseOutgoing("pending-b"); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("acknowledged outgoing source retained")
	}
	s.mu.Lock()
	s.expires = time.Now().Add(-time.Second)
	s.mu.Unlock()
	if n, err := reader.Read(part); n != 0 || err == nil {
		t.Fatal("expired source returned plaintext")
	}
}

func TestScratchRandomAccessSparseAndEncryptedStage(t *testing.T) {
	s, _ := fixture(t)
	id, err := s.BeginScratch("相机.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.WriteScratch(id, ScratchPageSize-2, []byte("ABCDEF")); err != nil {
		t.Fatal(err)
	}
	if err = s.WriteScratch(id, 0, []byte("ftyp")); err != nil {
		t.Fatal(err)
	}
	data, err := s.ReadScratch(id, ScratchPageSize-4, 10)
	if err != nil || !bytes.Equal(data, []byte{0, 0, 'A', 'B', 'C', 'D', 'E', 'F'}) {
		t.Fatal("sparse/random scratch mismatch", err)
	}
	path, err := s.StageScratch(id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ScratchSize(id); err == nil {
		t.Fatal("staged scratch retained")
	}
	reader, err := OpenOutgoing(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	head := make([]byte, 4)
	_, err = io.ReadFull(reader, head)
	if err != nil || string(head) != "ftyp" {
		t.Fatal("scratch stage altered bytes")
	}
}
