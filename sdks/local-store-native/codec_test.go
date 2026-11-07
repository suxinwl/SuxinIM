package localstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testKey = bytes.Repeat([]byte{0x42}, 32)
var testIdentity = ObjectIdentity{"emp_synthetic", "synthetic-object", "attachment"}

func encrypted(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := EncryptWithRandom(&out, bytes.NewReader(data), testKey, testIdentity, bytes.NewReader([]byte{0, 1, 2, 3, 4, 5, 6, 7})); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRoundTrips(t *testing.T) {
	for _, size := range []int{0, 1, 15, 16, 17, ChunkSize - 1, ChunkSize, ChunkSize + 1, ChunkSize*2 + 19} {
		t.Run(string(rune(size+65)), func(t *testing.T) {
			plain := bytes.Repeat([]byte{byte(size % 251)}, size)
			sealed := encrypted(t, plain)
			var out bytes.Buffer
			if err := Decrypt(&out, bytes.NewReader(sealed), testKey, testIdentity); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(plain, out.Bytes()) {
				t.Fatal("payload mismatch")
			}
			if bytes.HasPrefix(sealed, []byte("SQLite format 3")) {
				t.Fatal("plaintext output")
			}
		})
	}
}

func TestRejectsTruncationAndTrailingData(t *testing.T) {
	sealed := encrypted(t, []byte("synthetic-only"))
	for index := 0; index < len(sealed); index++ {
		if err := Decrypt(io.Discard, bytes.NewReader(sealed[:index]), testKey, testIdentity); !errors.Is(err, ErrInvalidObject) {
			t.Fatalf("accepted truncation %d", index)
		}
	}
	if Decrypt(io.Discard, bytes.NewReader(append(sealed, 0)), testKey, testIdentity) == nil {
		t.Fatal("accepted bytes after EOF")
	}
}

func TestRejectsTamperEveryByte(t *testing.T) {
	sealed := encrypted(t, []byte("synthetic-only"))
	for index := range sealed {
		changed := bytes.Clone(sealed)
		changed[index] ^= 1
		if Decrypt(io.Discard, bytes.NewReader(changed), testKey, testIdentity) == nil {
			t.Fatalf("accepted tamper %d", index)
		}
	}
}

func TestRejectsWrongIdentityOrKey(t *testing.T) {
	sealed := encrypted(t, []byte("synthetic-only"))
	for _, id := range []ObjectIdentity{{"other", testIdentity.ObjectID, testIdentity.Type}, {testIdentity.Account, "other", testIdentity.Type}, {testIdentity.Account, testIdentity.ObjectID, "other"}} {
		if Decrypt(io.Discard, bytes.NewReader(sealed), testKey, id) == nil {
			t.Fatal("cross-context object accepted")
		}
	}
	if Decrypt(io.Discard, bytes.NewReader(sealed), bytes.Repeat([]byte{7}, 32), testIdentity) == nil {
		t.Fatal("wrong key accepted")
	}
}

func TestRejectsReorderedFrames(t *testing.T) {
	sealed := encrypted(t, bytes.Repeat([]byte{1}, ChunkSize*2))
	start := 12 + int(binary.BigEndian.Uint32(sealed[8:12]))
	frameBytes := 4 + ChunkSize + 16
	changed := bytes.Clone(sealed)
	copy(changed[start:start+frameBytes], sealed[start+frameBytes:start+frameBytes*2])
	copy(changed[start+frameBytes:start+frameBytes*2], sealed[start:start+frameBytes])
	if Decrypt(io.Discard, bytes.NewReader(changed), testKey, testIdentity) == nil {
		t.Fatal("reordered frames accepted")
	}
}

func TestRejectsMissingRandomAndMalformedIdentity(t *testing.T) {
	if EncryptWithRandom(io.Discard, bytes.NewReader(nil), testKey, testIdentity, bytes.NewReader(nil)) == nil {
		t.Fatal("missing CSPRNG accepted")
	}
	for _, id := range []ObjectIdentity{{"", "a", "b"}, {"a\x00b", "c", "d"}, {"a", "b", ""}} {
		if Encrypt(io.Discard, bytes.NewReader(nil), testKey, id) == nil {
			t.Fatal("bad identity accepted")
		}
	}
}

func TestRealSourceFilenameAndWideCollisionSuffix(t *testing.T) {
	for _, item := range []struct {
		source string
		n      uint32
		want   string
	}{{"职员名单.xlsx", 0, "职员名单.xlsx"}, {"职员名单.xlsx", 1, "职员名单（1）.xlsx"}, {"职员名单.xlsx", 2, "职员名单（2）.xlsx"}, {"无扩展", 1, "无扩展（1）"}, {".config", 1, ".config（1）"}, {"原名（1）.pdf", 1, "原名（1）（1）.pdf"}} {
		got, err := CollisionName(item.source, item.n)
		if err != nil || got != item.want {
			t.Fatalf("name=%q error=%v", got, err)
		}
	}
	for _, bad := range []string{"", "../escape.txt", `..\escape.txt`, "CON.txt", "NUL", "file.", "file ", "a:b.txt", "a\x00.txt"} {
		if ValidateSourceName(bad) == nil {
			t.Fatalf("accepted unsafe name %q", bad)
		}
	}
}

func TestConcurrentReservationsNeverOverwrite(t *testing.T) {
	dir := t.TempDir()
	var group sync.WaitGroup
	names := make(chan string, 32)
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			r, err := ReserveName(dir, "职员名单.xlsx")
			if err != nil {
				failures <- err
				return
			}
			if err = Encrypt(r.File, bytes.NewReader([]byte("synthetic-only")), testKey, testIdentity); err == nil {
				err = r.Commit()
			}
			if err != nil {
				failures <- err
				return
			}
			names <- r.Name
		}()
	}
	group.Wait()
	close(names)
	close(failures)
	for err := range failures {
		t.Error(err)
	}
	seen := map[string]bool{}
	for name := range names {
		if seen[name] {
			t.Fatal("duplicate allocation")
		}
		seen[name] = true
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = Decrypt(io.Discard, bytes.NewReader(data), testKey, testIdentity); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 32 || !seen["职员名单.xlsx"] || !seen["职员名单（31）.xlsx"] {
		t.Fatal("missing source-name allocations")
	}
}

func TestReservationAbortRemovesOnlyOwnedFile(t *testing.T) {
	dir := t.TempDir()
	r, err := ReserveName(dir, "source.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Abort(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dir, "source.pdf")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reservation not removed")
	}
}

func TestSharedPCFormatVectors(t *testing.T) {
	data, err := os.ReadFile("../local-store-fixtures/format-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var vector struct {
		Format                        string
		SyntheticOnly                 bool
		MasterHex, NoncePrefixHex     string
		Context                       ObjectIdentity
		PlainBase64, CiphertextBase64 string
		LargeVector                   struct {
			PlainByte        byte
			PlainLength      int
			CiphertextSHA256 string
		}
	}
	if err = json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	if !vector.SyntheticOnly || vector.Format != Format {
		t.Fatal("not an approved synthetic format vector")
	}
	key, err := hex.DecodeString(vector.MasterHex)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := hex.DecodeString(vector.NoncePrefixHex)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := base64.StdEncoding.DecodeString(vector.PlainBase64)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := base64.StdEncoding.DecodeString(vector.CiphertextBase64)
	if err != nil {
		t.Fatal(err)
	}
	var out, restored bytes.Buffer
	if err = EncryptWithRandom(&out, bytes.NewReader(plain), key, vector.Context, bytes.NewReader(prefix)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out.Bytes(), expected) {
		t.Fatal("PC/Go ciphertext bytes differ")
	}
	if err = Decrypt(&restored, bytes.NewReader(expected), key, vector.Context); err != nil || !bytes.Equal(restored.Bytes(), plain) {
		t.Fatal("PC ciphertext cannot be decrypted")
	}
	out.Reset()
	large := bytes.Repeat([]byte{vector.LargeVector.PlainByte}, vector.LargeVector.PlainLength)
	if err = EncryptWithRandom(&out, bytes.NewReader(large), key, vector.Context, bytes.NewReader(prefix)); err != nil {
		t.Fatal(err)
	}
	got := sha256.Sum256(out.Bytes())
	if hex.EncodeToString(got[:]) != vector.LargeVector.CiphertextSHA256 {
		t.Fatal("PC/Go multi-frame ciphertext differs")
	}
}
