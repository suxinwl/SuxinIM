package builtinai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareMenuSkipsDatabaseBeforeInstallation(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "not-created", "install.lock")
	calls := 0
	err := prepareMenu(context.Background(), lock, func(context.Context) error {
		calls++
		return errors.New("database is not initialized")
	})
	if err != nil || calls != 0 {
		t.Fatalf("missing installation lock must skip database initialization: calls=%d err=%v", calls, err)
	}
}

func TestPrepareMenuInitializesInstalledSystemAndPropagatesErrors(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "install.lock")
	if err := os.WriteFile(lock, nil, 0600); err != nil {
		t.Fatal(err)
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "installer-ready")
	for _, expected := range []error{nil, errors.New("menu database unavailable")} {
		calls := 0
		err := prepareMenu(ctx, lock, func(actual context.Context) error {
			calls++
			if actual != ctx || actual.Value(contextKey{}) != "installer-ready" {
				t.Fatal("menu initialization lost its request context")
			}
			return expected
		})
		if calls != 1 || err != expected {
			t.Fatalf("installed system must initialize once and preserve errors: calls=%d got=%v expected=%v", calls, err, expected)
		}
	}
}

func TestPrepareMenuRejectsDirectoryMarker(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "install.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := prepareMenu(context.Background(), lock, func(context.Context) error { calls++; return nil })
	if err == nil || !strings.Contains(err.Error(), "安装锁必须为普通文件") || calls != 0 {
		t.Fatalf("directory installation lock accepted: calls=%d err=%v", calls, err)
	}
}

func TestPrepareMenuPreservesOtherStatErrors(t *testing.T) {
	// A NUL is invalid in a filename on both Windows and Unix, regardless of
	// whether the current user can bypass file permission checks.
	lock := filepath.Join(t.TempDir(), "invalid\x00install.lock")
	_, statErr := os.Stat(lock)
	if statErr == nil || os.IsNotExist(statErr) {
		t.Fatalf("fixture must cause a stat failure other than missing file: %v", statErr)
	}
	calls := 0
	err := prepareMenu(context.Background(), lock, func(context.Context) error { calls++; return nil })
	var pathErr *os.PathError
	if err == nil || calls != 0 || !errors.As(err, &pathErr) || pathErr.Path != lock {
		t.Fatalf("stat error was swallowed or initialization ran: calls=%d err=%v", calls, err)
	}
}
