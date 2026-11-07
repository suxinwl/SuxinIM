package builtinai

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/suxinwl/GoSuxin/internal/devtools/ai"
)

func legacyAI(t *testing.T, root string) (*ai.Service, ai.Token, string) {
	t.Helper()
	s, err := ai.NewService(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveConfig(7, ai.Config{Endpoint: "https://ai.example.invalid/v1", Model: "migration-model", APIKey: "test-only-config-secret"}, false); err != nil {
		t.Fatal(err)
	}
	metadata, raw, err := s.CreateToken(7, "迁移测试", []string{"ai:generate"}, 7)
	if err != nil {
		t.Fatal(err)
	}
	return s, metadata, raw
}

func assertNoAIStages(t *testing.T, parent string) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(parent, ".ai-migrate-*"))
	if err != nil || len(stages) != 0 {
		t.Fatal("migration stage was not cleaned")
	}
}

func TestMigrateLegacyAIPreservesCiphertextTokensSourceAndPermissions(t *testing.T) {
	base := t.TempDir()
	source, target := filepath.Join(base, "legacy"), filepath.Join(base, "system", "ai")
	_, token, raw := legacyAI(t, source)
	keyBefore, err := os.ReadFile(filepath.Join(source, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := os.ReadFile(filepath.Join(source, "config", "7.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyAI(source, target); err != nil {
		t.Fatal(err)
	}
	keyAfter, err := os.ReadFile(filepath.Join(target, "master.key"))
	if err != nil || !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("master key changed during migration")
	}
	configAfter, err := os.ReadFile(filepath.Join(target, "config", "7.json"))
	if err != nil || !bytes.Equal(configBefore, configAfter) {
		t.Fatal("encrypted configuration changed during migration")
	}
	var config ai.Config
	if err := json.Unmarshal(configAfter, &config); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(keyAfter)
	if err != nil {
		t.Fatal("migrated key invalid")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := base64.RawStdEncoding.DecodeString(config.SealedKey)
	if err != nil || len(sealed) < gcm.NonceSize() {
		t.Fatal("migrated encrypted config invalid")
	}
	plain, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
	if err != nil || string(plain) != "test-only-config-secret" {
		t.Fatal("migrated encrypted config cannot be decrypted")
	}
	migrated, err := ai.NewService(target)
	if err != nil {
		t.Fatal(err)
	}
	got, err := migrated.ValidateToken(raw)
	if err != nil || got.ID != token.ID || got.UID != 7 || !got.Allows("ai:generate") {
		t.Fatal("existing developer token no longer validates")
	}
	if sourceKey, err := os.ReadFile(filepath.Join(source, "master.key")); err != nil || !bytes.Equal(sourceKey, keyBefore) {
		t.Fatal("source key was modified")
	}
	if sourceConfig, err := os.ReadFile(filepath.Join(source, "config", "7.json")); err != nil || !bytes.Equal(sourceConfig, configBefore) {
		t.Fatal("source configuration was modified")
	}
	if _, err := os.Stat(filepath.Join(source, "tokens", token.ID+".json")); err != nil {
		t.Fatal("source token was removed")
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{target, filepath.Join(target, "config"), filepath.Join(target, "tokens")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0700 {
				t.Fatal("destination directory permissions are not private")
			}
		}
		for _, path := range []string{filepath.Join(target, "master.key"), filepath.Join(target, "config", "7.json"), filepath.Join(target, "tokens", token.ID+".json")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("destination file permissions are not private")
			}
		}
	}
	assertNoAIStages(t, filepath.Dir(target))
}

func TestExistingSystemAIIsAuthoritativeAndNotOverwritten(t *testing.T) {
	base := t.TempDir()
	source, target := filepath.Join(base, "legacy"), filepath.Join(base, "system", "ai")
	_, oldToken, oldRaw := legacyAI(t, source)
	current, err := ai.NewService(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.SaveConfig(7, ai.Config{Endpoint: "https://new.example.invalid/v1", Model: "system-authority", APIKey: "new-test-secret"}, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(target, "config", "7.json"))
	if err != nil {
		t.Fatal(err)
	}
	masterBefore, err := os.ReadFile(filepath.Join(target, "master.key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyAI(source, target); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(target, "config", "7.json"))
	masterAfter, _ := os.ReadFile(filepath.Join(target, "master.key"))
	if !bytes.Equal(before, after) || !bytes.Equal(masterBefore, masterAfter) {
		t.Fatal("existing destination data was overwritten")
	}
	if _, err := os.Stat(filepath.Join(target, "tokens", oldToken.ID+".json")); !os.IsNotExist(err) {
		t.Fatal("legacy token merged into authoritative destination")
	}
	if _, err := current.ValidateToken(oldRaw); err == nil {
		t.Fatal("legacy token unexpectedly accepted by existing destination")
	}
	// An existing target must not inspect or depend on missing legacy data.
	if err := MigrateLegacyAI(filepath.Join(base, "absent"), target); err != nil {
		t.Fatal("existing target should be a no-op without legacy source")
	}
	assertNoAIStages(t, filepath.Dir(target))
}

func TestMigrationTargetEmptyNonemptyAndMissingLegacy(t *testing.T) {
	t.Run("empty target", func(t *testing.T) {
		base := t.TempDir()
		source, target := filepath.Join(base, "old"), filepath.Join(base, "new")
		legacyAI(t, source)
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := MigrateLegacyAI(source, target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(target, "master.key")); err != nil {
			t.Fatal("empty target not populated")
		}
		assertNoAIStages(t, filepath.Dir(target))
	})
	t.Run("nonempty target without key", func(t *testing.T) {
		base := t.TempDir()
		source, target := filepath.Join(base, "old"), filepath.Join(base, "new")
		legacyAI(t, source)
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(target, "config.json")
		before := []byte("existing-system-data")
		if err := os.WriteFile(path, before, 0600); err != nil {
			t.Fatal(err)
		}
		if err := MigrateLegacyAI(source, target); err == nil {
			t.Fatal("nonempty keyless target accepted")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("existing target was modified")
		}
		assertNoAIStages(t, filepath.Dir(target))
	})
	t.Run("missing legacy", func(t *testing.T) {
		base := t.TempDir()
		target := filepath.Join(base, "new")
		if err := MigrateLegacyAI(filepath.Join(base, "missing"), target); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("missing legacy migration created destination")
		}
	})
}

func TestMigrationRejectsSameNestedAndInvalidPaths(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "legacy")
	legacyAI(t, source)
	for _, paths := range [][2]string{
		{source, source},
		{source, filepath.Join(source, "new")},
		{source, base},
		{"", filepath.Join(base, "new")},
		{source, ""},
	} {
		if err := MigrateLegacyAI(paths[0], paths[1]); err == nil {
			t.Fatal("invalid source/destination relation accepted")
		}
	}
	if runtime.GOOS == "windows" && MigrateLegacyAI(source, filepath.Clean(filepath.Dir(source))+string(filepath.Separator)+"LEGACY") == nil {
		t.Fatal("case-insensitive same path accepted")
	}
}

func TestMigrationRejectsOversizedAndIncompleteLegacyCleansStage(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		base := t.TempDir()
		source, target := filepath.Join(base, "legacy"), filepath.Join(base, "new")
		legacyAI(t, source)
		path := filepath.Join(source, "tokens", "oversized.json")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		err = f.Truncate(maxMigrationFileBytes + 1)
		closeErr := f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		if err := MigrateLegacyAI(source, target); err == nil {
			t.Fatal("oversized legacy file accepted")
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("failed migration published destination")
		}
		if _, err := os.Stat(filepath.Join(source, "master.key")); err != nil {
			t.Fatal("failure changed source")
		}
		assertNoAIStages(t, filepath.Dir(target))
	})
	t.Run("incomplete key", func(t *testing.T) {
		base := t.TempDir()
		source, target := filepath.Join(base, "legacy"), filepath.Join(base, "new")
		legacyAI(t, source)
		if err := os.WriteFile(filepath.Join(source, "master.key"), []byte("incomplete"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := MigrateLegacyAI(source, target); err == nil {
			t.Fatal("incomplete legacy key accepted")
		}
		if _, err := os.Stat(target); !os.IsNotExist(err) {
			t.Fatal("invalid key published")
		}
		assertNoAIStages(t, filepath.Dir(target))
	})
}

func TestMigrationRejectsSourceTargetAndNestedSymlinks(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "legacy")
	legacyAI(t, source)
	sourceLink := filepath.Join(base, "legacy-link")
	if err := os.Symlink(source, sourceLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := MigrateLegacyAI(sourceLink, filepath.Join(base, "new")); err == nil {
		t.Fatal("source symlink accepted")
	}
	targetReal := filepath.Join(base, "real-target")
	if err := os.Mkdir(targetReal, 0700); err != nil {
		t.Fatal(err)
	}
	targetLink := filepath.Join(base, "target-link")
	if err := os.Symlink(targetReal, targetLink); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyAI(source, filepath.Join(targetLink, "ai")); err == nil {
		t.Fatal("target ancestor symlink accepted")
	}
	if err := os.Symlink(filepath.Join(source, "master.key"), filepath.Join(source, "tokens", "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := MigrateLegacyAI(source, filepath.Join(base, "new")); err == nil {
		t.Fatal("nested source symlink accepted")
	}
	if _, err := os.Stat(filepath.Join(base, "new")); !os.IsNotExist(err) {
		t.Fatal("symlink rejection published destination")
	}
	assertNoAIStages(t, base)
}

func TestMigrationRejectsSpecialFile(t *testing.T) {
	base := t.TempDir()
	source, target := filepath.Join(base, "old"), filepath.Join(base, "new")
	legacyAI(t, source)
	socket := filepath.Join(source, "tokens", "s.json")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("Unix socket unavailable on this platform/path: %v", err)
	}
	defer listener.Close()
	if err := MigrateLegacyAI(source, target); err == nil {
		t.Fatal("special file accepted")
	}
	assertNoAIStages(t, filepath.Dir(target))
}

func TestDefaultServiceUsesSystemOverrideMigratesOnceAndKeepsSource(t *testing.T) {
	// DefaultService is process-wide. Keep this test sequential and restore the
	// original singleton so other package tests never inherit its temporary root.
	previous := systemAI
	systemAI = &serviceState{}
	t.Cleanup(func() { systemAI = previous })
	base := t.TempDir()
	t.Chdir(base)
	source := filepath.Join(base, "storage", "plugins", "devtools", "data", "ai")
	_, token, raw := legacyAI(t, source)
	systemRoot := filepath.Join(base, "system-override")
	t.Setenv("SUXIN_SYSTEM_DATA", systemRoot)
	first, err := DefaultService()
	if err != nil {
		t.Fatal(err)
	}
	second, err := DefaultService()
	if err != nil || first != second {
		t.Fatal("default service was not initialized once")
	}
	if _, err := os.Stat(filepath.Join(systemRoot, "ai", "master.key")); err != nil {
		t.Fatal("system override not used")
	}
	if got, err := first.ValidateToken(raw); err != nil || got.ID != token.ID {
		t.Fatal("default service did not migrate existing token")
	}
	if _, err := os.Stat(filepath.Join(source, "master.key")); err != nil {
		t.Fatal("default migration removed original data")
	}
}
