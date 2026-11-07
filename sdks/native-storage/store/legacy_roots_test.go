package nativestore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyRootAliasAllowsOnlyDeclaredSystemAlias(t *testing.T) {
	s, _ := fixture(t)
	base := t.TempDir()
	actual, alias := filepath.Join(base, "actual"), filepath.Join(base, "declared")
	if err := os.MkdirAll(filepath.Join(actual, "owned"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, alias); err != nil {
		t.Fatal(err)
	}
	actual = filepath.Join(actual, "owned")
	alias = filepath.Join(alias, "owned")
	if err := os.WriteFile(filepath.Join(actual, "owned.bin"), []byte("SYNTHETIC"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLegacyRoots([]string{alias}); err != nil {
		t.Fatal(err)
	}
	if !s.legacyOwned(filepath.Join(alias, "owned.bin")) {
		t.Fatal("declared native root alias rejected")
	}
	if s.legacyOwned(filepath.Join(actual, "owned.bin")) {
		t.Fatal("undeclared path spelling accepted")
	}
	if err := os.Symlink(filepath.Join(actual, "owned.bin"), filepath.Join(actual, "linked.bin")); err != nil {
		t.Fatal(err)
	}
	if s.legacyOwned(filepath.Join(alias, "linked.bin")) {
		t.Fatal("additional child symlink accepted")
	}
	outside := filepath.Join(base, "outside.bin")
	if err := os.WriteFile(outside, []byte("SYNTHETIC"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(actual, "escape.bin")); err != nil {
		t.Fatal(err)
	}
	if s.legacyOwned(filepath.Join(alias, "escape.bin")) {
		t.Fatal("escaping child symlink accepted")
	}
}

func TestLegacyRootExcludesAllProtectedAccountRoots(t *testing.T) {
	s, _ := fixture(t)
	base := t.TempDir()
	s.root = filepath.Join(base, "PUTY-Local", "accounts", "synthetic-a")
	other := filepath.Join(base, "PUTY-Local", "accounts", "synthetic-b", "private.bin")
	if err := os.MkdirAll(filepath.Dir(other), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("SYNTHETIC"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetLegacyRoots([]string{base}); err != nil {
		t.Fatal(err)
	}
	if s.legacyOwned(other) {
		t.Fatal("another account protected vault treated as plaintext legacy data")
	}
}
