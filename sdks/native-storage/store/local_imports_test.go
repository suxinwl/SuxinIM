package nativestore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExplicitLocalImportsUseIndependentReferencesAndPreciseClearCounts(t *testing.T) {
	s, _ := fixture(t)
	at := time.Now().UnixMilli()
	insert(t, s, "ordinary", "SYNTHETIC", at)
	insert(t, s, "private", "SYNTHETIC_PRIVATE", at)
	if _, err := s.db.Exec(`UPDATE chat_logs_synthetic SET attached_info='{"isPrivateChat":true}' WHERE client_msg_id='private'`); err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(100); err != nil {
		t.Fatal(err)
	}
	body := []byte("SYNTHETIC_IMPORT_BODY")
	var imported []AttachmentItem
	for i := 0; i < 2; i++ {
		path, err := s.StageOutgoing(bytes.NewReader(body), "职员名单.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		item, err := s.ImportLocalAttachment(path, "synthetic")
		if err != nil {
			t.Fatal(err)
		}
		if item.LocalImportID == "" || item.MessageID != "" || item.ConversationID != "synthetic" || item.FirstImportedAt < at || item.Size != int64(len(body)) {
			t.Fatal("local import impersonated an SDK message")
		}
		imported = append(imported, item)
	}
	if imported[0].Name != "职员名单.xlsx" || imported[1].Name != "职员名单（1）.xlsx" {
		t.Fatal("actual encrypted names not preserved")
	}
	if _, err := s.BeginAttachment("shared", "synthetic-shared-object", "共享.bin", "synthetic", "ordinary"); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteAttachment("shared", []byte("SYNTHETIC_SHARED")); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAttachment("shared", 16); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginAttachment("shared", "synthetic-shared-object", "共享.bin", "outside", "outside-msg"); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewClear("chat_logs_synthetic", "synthetic", at-1, time.Now().Add(time.Second).UnixMilli())
	if err != nil {
		t.Fatal(err)
	}
	if preview.MessageCount != 2 || preview.LocalImportCount != 2 || preview.AttachmentReferences != 3 || preview.FilesToDelete != 2 {
		t.Fatalf("wrong cleanup counts: %+v", preview)
	}
	page, err := s.AttachmentInventory("synthetic", "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var ownImports int
	for _, item := range page.Items {
		if item.LocalImportID != "" {
			ownImports++
			if item.MessageID != "" {
				t.Fatal("inventory invented SDK ID")
			}
		}
	}
	if ownImports != 2 {
		t.Fatal("missing local import inventory")
	}
	if err = s.ClearRange("chat_logs_synthetic", "synthetic", preview.From, preview.To); err != nil {
		t.Fatal(err)
	}
	for _, item := range imported {
		if _, err = os.Stat(filepath.Join(s.root, "archive", item.Month, "attachments", item.Name)); !os.IsNotExist(err) {
			t.Fatal("local import file survived exact scoped clear", err)
		}
	}
	if count(t, s, `SELECT count(*) FROM puty_local_imports`) != 0 {
		t.Fatal("local import metadata survived clear")
	}
	retained, err := s.ReadAttachment("shared", 1024)
	if err != nil || string(retained) != "SYNTHETIC_SHARED" {
		t.Fatal("shared outside reference was removed", err)
	}
}

func TestLocalImportRequiresConversationAndOwnEncryptedSource(t *testing.T) {
	s, _ := fixture(t)
	external := filepath.Join(t.TempDir(), "original.txt")
	if err := os.WriteFile(external, []byte("SYNTHETIC_EXTERNAL_ORIGINAL"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportLocalAttachment(external, "synthetic"); err == nil {
		t.Fatal("arbitrary plaintext path accepted")
	}
	path, err := s.StageOutgoing(bytes.NewReader([]byte("SYNTHETIC")), "original.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportLocalAttachment(path, ""); err == nil {
		t.Fatal("unclassified conversation accepted")
	}
	if _, err = s.ImportLocalAttachment(path, "synthetic"); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(external)
	if err != nil || string(original) != "SYNTHETIC_EXTERNAL_ORIGINAL" {
		t.Fatal("external original deleted", err)
	}
}
