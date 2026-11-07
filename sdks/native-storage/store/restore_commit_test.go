package nativestore

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	localstore "puty.local/localstore"
)

func TestRestorePowerLossJournalReconstructsEveryRenameBoundary(t *testing.T) {
	for _, state := range []string{"before-first", "between-renames", "after-second", "stage-lost", "tampered-stage", "unsafe-root"} {
		t.Run(state, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "account")
			stage := root + ".restore-00112233445566778899aabb"
			previous := root + ".previous-00112233445566778899aacc"
			for _, dir := range []string{root, stage} {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "working.db"), []byte("SYNTHETIC_OLD_CIPHER"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(stage, "working.db"), []byte("SYNTHETIC_NEW_CIPHER"), 0600); err != nil {
				t.Fatal(err)
			}
			oldHash, err := hashOwnedTree(root, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			stageHash, err := hashOwnedTree(stage, func() error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			record := promotion{filepath.Base(root), filepath.Base(stage), filepath.Base(previous), oldHash, stageHash}
			if err = writePromotion(root, record); err != nil {
				t.Fatal(err)
			}
			if state != "before-first" {
				if err = os.Rename(root, previous); err != nil {
					t.Fatal(err)
				}
				if err = syncDirectory(parent); err != nil {
					t.Fatal(err)
				}
			}
			switch state {
			case "after-second":
				if err = os.Rename(stage, root); err != nil {
					t.Fatal(err)
				}
			case "stage-lost":
				if err = os.RemoveAll(stage); err != nil {
					t.Fatal(err)
				}
			case "tampered-stage":
				if err = os.WriteFile(filepath.Join(stage, "working.db"), []byte("TAMPERED"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unsafe-root":
				if err = os.WriteFile(root, []byte("not-a-directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = RecoverPromotion(root)
			if state == "tampered-stage" || state == "unsafe-root" {
				if err == nil {
					t.Fatal("unsafe power-loss state silently accepted")
				}
				if _, e := os.Stat(root + ".promotion"); e != nil {
					t.Fatal("failed recovery lost journal")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(root + ".promotion"); !os.IsNotExist(err) {
				t.Fatal("completed recovery retained journal")
			}
			body, err := os.ReadFile(filepath.Join(root, "working.db"))
			if err != nil {
				t.Fatal(err)
			}
			expected := "SYNTHETIC_NEW_CIPHER"
			if state == "before-first" || state == "stage-lost" {
				expected = "SYNTHETIC_OLD_CIPHER"
			}
			if string(body) != expected {
				t.Fatal("wrong recovered root")
			}
		})
	}
}

func TestRestoreGCReplayedReceiptPersistentSyncFailureNeverUnlinks(t *testing.T) {
	s, root := fixture(t)
	previous := root + ".previous-00112233445566778899aabb"
	if err := os.Mkdir(previous, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(previous, "remaining")
	if err := os.WriteFile(source, []byte("SYNTHETIC_MUST_REMAIN"), 0600); err != nil {
		t.Fatal(err)
	}
	receipt, err := s.buildRestoreGarbage(previous, root, filepath.Base(previous), s.master, s.checkAuthorized)
	if err != nil {
		t.Fatal(err)
	}
	receipt.Phase = "deleting"
	encoded, _ := json.Marshal(receipt)
	if err = atomicEncrypted(filepath.Join(root, ".restore-gc"), encoded, s.master, localstore.ObjectIdentity{Account: s.account, Type: "restore-gc", ObjectID: "previous"}); err != nil {
		t.Fatal(err)
	}
	original := gcDurabilityBoundary
	defer func() { gcDurabilityBoundary = original }()
	gcDurabilityBoundary = func(*Store, restoreGarbage, []byte) error { return syscall.EIO }
	for attempt := 0; attempt < 2; attempt++ {
		if err = s.FinalizeRestoredPrevious(); err != syscall.EIO {
			t.Fatal("durability failure ignored", err)
		}
		if _, err = os.Stat(source); err != nil {
			t.Fatal("persistent fsync fault deleted a source", err)
		}
	}
	gcDurabilityBoundary = original
	if err = s.FinalizeRestoredPrevious(); err != nil {
		t.Fatal(err)
	}
}

func TestRestorePreviousRequiresFreshSyncAndExactDurableGC(t *testing.T) {
	s, root := fixture(t)
	insert(t, s, "record", "SYNTHETIC_RESTORE_GC", 10)
	output, err := s.ExportBackup("synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	if err = Configure(s.account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	prepared, err := s.PrepareRestore(output, "synthetic recovery password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Abort()
	if err = prepared.PrepareCommit(bytes.Repeat([]byte{0x42}, 64)); err != nil {
		t.Fatal(err)
	}
	// Preparing closes the old DB but still permits the same online actor's
	// renewal. The final transition has no long tree hash or SQL integrity work.
	if err = Configure(s.account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err = prepared.Commit(bytes.Repeat([]byte{0x42}, 64)); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(filepath.Dir(root), prepared.commitRecord.Previous)
	account := s.account
	Lock(account)
	if err = Configure(account, root, bytes.Repeat([]byte{0x71}, 32), time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	restored, err := Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = restored.OpenWorkingDatabase(); err != nil {
		t.Fatal(err)
	}
	if err = restored.FinalizeRestoredPrevious(); err != ErrLocked {
		t.Fatal("old vault collected before fresh sync", err)
	}
	if _, err = os.Stat(previous); err != nil {
		t.Fatal("pre-sync recovery copy missing", err)
	}
	restored.MarkViewsReady(true)
	// A changed old file must never be deleted merely by matching its basename.
	extra := filepath.Join(previous, "unexpected")
	if err = os.WriteFile(extra, []byte("SYNTHETIC_NOT_IN_RECEIPT"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = restored.FinalizeRestoredPrevious(); err == nil {
		t.Fatal("changed previous tree bypassed hash")
	}
	if err = os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if err = restored.FinalizeRestoredPrevious(); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(previous); !os.IsNotExist(err) {
		t.Fatal("fresh synced previous vault survived GC", err)
	}
	if _, err = os.Stat(filepath.Join(root, ".restore-gc")); !os.IsNotExist(err) {
		t.Fatal("GC receipt not acknowledged", err)
	}
	if err = restored.FinalizeRestoredPrevious(); err != nil {
		t.Fatal("GC retry not idempotent", err)
	}
}

func TestRestoreGCDeletingReceiptResumesOnlyExactRemainingSubset(t *testing.T) {
	for _, mutation := range []string{"none", "unknown-file", "replaced-root"} {
		t.Run(mutation, func(t *testing.T) {
			s, root := fixture(t)
			previous := root + ".previous-00112233445566778899aabb"
			if err := os.Mkdir(previous, 0700); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"removed", "remaining"} {
				if err := os.WriteFile(filepath.Join(previous, name), []byte("SYNTHETIC_CIPHER_"+name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			receipt, err := s.buildRestoreGarbage(previous, root, filepath.Base(previous), s.master, s.checkAuthorized)
			if err != nil {
				t.Fatal(err)
			}
			receipt.Phase = "deleting"
			encoded, _ := json.Marshal(receipt)
			if err = atomicEncrypted(filepath.Join(root, ".restore-gc"), encoded, s.master, localstore.ObjectIdentity{Account: s.account, Type: "restore-gc", ObjectID: "previous"}); err != nil {
				t.Fatal(err)
			}
			if err = os.Remove(filepath.Join(previous, "removed")); err != nil {
				t.Fatal(err)
			}
			if mutation == "unknown-file" {
				if err = os.WriteFile(filepath.Join(previous, "UNKNOWN"), []byte("SYNTHETIC_UNKNOWN"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "replaced-root" {
				if err = os.Rename(previous, previous+".hold"); err != nil {
					t.Fatal(err)
				}
				if err = os.Mkdir(previous, 0700); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(previous, "remaining"), []byte("SYNTHETIC_CIPHER_remaining"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = s.FinalizeRestoredPrevious()
			if mutation != "none" {
				if err == nil {
					t.Fatal("unknown/replaced old tree was claimed")
				}
				if _, e := os.Stat(filepath.Join(previous, "remaining")); e != nil {
					t.Fatal("failure deleted an additional file", e)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(previous); !os.IsNotExist(err) {
				t.Fatal("partial exact GC not resumed", err)
			}
		})
	}
}
