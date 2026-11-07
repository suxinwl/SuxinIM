package nativestore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	localstore "puty.local/localstore"
)

type promotion struct {
	Root           string `json:"root"`
	Stage          string `json:"stage"`
	Previous       string `json:"previous"`
	PreviousSHA256 string `json:"previousSHA256"`
	StageSHA256    string `json:"stageSHA256"`
}

func (p *PreparedRestore) check() error {
	if p.cancelled.Load() {
		return errors.New("restore cancelled")
	}
	return p.owner.checkAuthorized()
}

// Long verification and hashing run while the native broker may renew this
// same actor's lease. The SDK must already be quiesced. The old DB is closed
// before hashing; it can never receive an ACK/mutation after this point.
func (p *PreparedRestore) PrepareCommit(wrappedMaster []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || len(wrappedMaster) < 32 || len(wrappedMaster) > 4096 {
		return errors.New("invalid native restore handle")
	}
	hash := sha256.Sum256(wrappedMaster)
	if p.commitReady {
		if hash != p.wrappedHash {
			return errors.New("restore envelope changed")
		}
		return p.check()
	}
	if err := p.check(); err != nil {
		return err
	}
	p.owner.flushMu.Lock()
	defer p.owner.flushMu.Unlock()
	if err := p.verifyAndMerge(); err != nil {
		return err
	}
	if err := p.check(); err != nil {
		return err
	}
	p.owner.closeRangeReaders()
	p.preparedDB = p.owner.db
	if p.preparedDB != nil {
		if err := p.preparedDB.Close(); err != nil {
			return err
		}
	}
	if err := syncOwnedTree(p.owner.root, p.check); err != nil {
		return err
	}
	nonce, err := randomName()
	if err != nil {
		return err
	}
	previous := filepath.Base(p.owner.root) + ".previous-" + nonce
	file, err := os.OpenFile(filepath.Join(p.stage, "master.wrap"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(wrappedMaster); err == nil {
		err = file.Sync()
	}
	_ = file.Close()
	if err != nil {
		return err
	}
	garbage, err := p.owner.buildRestoreGarbage(p.owner.root, p.stage, previous, p.master, p.check)
	if err != nil {
		return err
	}
	previousHash := garbage.SHA256
	receipt, _ := json.Marshal(garbage)
	if err = atomicEncrypted(filepath.Join(p.stage, ".restore-gc"), receipt, p.master, localstore.ObjectIdentity{Account: p.owner.account, Type: "restore-gc", ObjectID: "previous"}); err != nil {
		return err
	}
	if err = syncOwnedTree(p.stage, p.check); err != nil {
		return err
	}
	stageHash, err := hashOwnedTree(p.stage, p.check)
	if err != nil {
		return err
	}
	p.commitRecord = promotion{filepath.Base(p.owner.root), filepath.Base(p.stage), previous, previousHash, stageHash}
	p.wrappedHash = hash
	p.commitReady = true
	return p.check()
}

// Only the short durable promotion is held inside the OS broker transition.
// Every rename has a durable parent receipt and a subsequent parent fsync.
func (p *PreparedRestore) Commit(wrappedMaster []byte) error {
	if err := p.PrepareCommit(wrappedMaster); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.preparedDB != p.owner.db {
		return errors.New("restore SDK was reopened")
	}
	if err := p.check(); err != nil {
		return err
	}
	root := p.owner.root
	parent := filepath.Dir(root)
	old := filepath.Join(parent, p.commitRecord.Previous)
	if err := writePromotion(root, p.commitRecord); err != nil {
		return err
	}
	if err := p.check(); err != nil {
		return err
	}
	if err := os.Rename(root, old); err != nil {
		return err
	}
	if err := syncDirectory(parent); err != nil {
		return err
	}
	if err := os.Rename(p.stage, root); err != nil {
		return err
	}
	if err := syncDirectory(parent); err != nil {
		return err
	}
	p.closed = true
	clear(p.master)
	if err := os.Remove(root + ".promotion"); err != nil {
		return err
	}
	return syncDirectory(parent)
}

func noncePath(value, prefix string) bool {
	return filepath.Base(value) == value && strings.HasPrefix(value, prefix) && len(value) == len(prefix)+24 && strings.Trim(strings.TrimPrefix(value, prefix), "0123456789abcdef") == ""
}
func hashText(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil && len(value) == 64 && strings.ToLower(value) == value
}
func writePromotion(root string, record promotion) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(root+".promotion", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	_ = file.Close()
	if err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(root))
}

// Hashes ciphertext and exact relative names, rejecting all symlinks. No names
// or business payload are exposed by the receipt (only the aggregate SHA256).
func hashOwnedTree(root string, check func() error) (string, error) {
	return walkHashedTree(root, check, nil)
}
func syncOwnedTree(root string, check func() error) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = check(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("restore tree contains symlink")
		}
		if entry.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("invalid restore file")
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		err = file.Sync()
		_ = file.Close()
		return err
	})
	if err != nil {
		return err
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if err = check(); err != nil {
			return err
		}
		if err = syncDirectory(dirs[i]); err != nil {
			return err
		}
	}
	return syncDirectory(filepath.Dir(root))
}

// Called before native master unwrap/mkdir. Reconstructs actual crash states,
// authenticating both complete directory trees against the durable receipt.
func RecoverPromotion(root string) error {
	data, err := os.ReadFile(root + ".promotion")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !filepath.IsAbs(root) || len(data) > 4096 {
		return errors.New("invalid restore journal")
	}
	var record promotion
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&record) != nil || decoder.Decode(new(any)) != io.EOF {
		return errors.New("invalid restore journal")
	}
	base := filepath.Base(root)
	if record.Root != base || !noncePath(record.Stage, base+".restore-") || !noncePath(record.Previous, base+".previous-") || !hashText(record.PreviousSHA256) || !hashText(record.StageSHA256) {
		return errors.New("unsafe restore journal")
	}
	// A previous write may have returned EIO after rename but before fsync.
	// Reading a receipt is insufficient: re-establish its durability before
	// making any further directory change during recovery.
	journal, err := os.OpenFile(root+".promotion", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	current, readErr := io.ReadAll(io.LimitReader(journal, 4097))
	if readErr == nil && !bytes.Equal(current, data) {
		readErr = errors.New("restore journal changed")
	}
	if readErr == nil {
		readErr = journal.Sync()
	}
	_ = journal.Close()
	if readErr != nil {
		return readErr
	}
	if err = syncDirectory(filepath.Dir(root)); err != nil {
		return err
	}
	parent := filepath.Dir(root)
	stage := filepath.Join(parent, record.Stage)
	previous := filepath.Join(parent, record.Previous)
	digest := func(path string) (string, error) {
		info, e := os.Lstat(path)
		if os.IsNotExist(e) {
			return "", nil
		}
		if e != nil {
			return "", e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("unsafe restore root")
		}
		return hashOwnedTree(path, func() error { return nil })
	}
	rootHash, err := digest(root)
	if err != nil {
		return err
	}
	oldHash, err := digest(previous)
	if err != nil {
		return err
	}
	stageHash, err := digest(stage)
	if err != nil {
		return err
	}
	if oldHash != "" && oldHash != record.PreviousSHA256 || stageHash != "" && stageHash != record.StageSHA256 {
		return errors.New("restore journal tree mismatch")
	}
	switch {
	case rootHash == record.StageSHA256: // new root durable; keep previous for fresh-sync GC
	case rootHash == record.PreviousSHA256 && oldHash == "": // crash before first rename / explicit rollback
		if stageHash != "" {
			if err = removeOwnedStaging(root, stage); err != nil {
				return err
			}
		}
	case rootHash == "" && oldHash == record.PreviousSHA256:
		if stageHash == record.StageSHA256 {
			err = os.Rename(stage, root)
		} else {
			err = os.Rename(previous, root)
		}
		if err != nil {
			return err
		}
	default:
		return errors.New("restore root cannot be recovered safely")
	}
	if err = syncDirectory(parent); err != nil {
		return err
	}
	if err = os.Remove(root + ".promotion"); err != nil {
		return err
	}
	return syncDirectory(parent)
}
