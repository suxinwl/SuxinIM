package admindeveloper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrontendPackageManager(t *testing.T) {
	for _, tc := range []struct {
		name  string
		locks []string
		yarn  bool
	}{
		{"no lock defaults to npm", nil, false},
		{"npm lock", []string{"package-lock.json"}, false},
		{"yarn lock", []string{"yarn.lock"}, true},
		{"both prefer npm", []string{"package-lock.json", "yarn.lock"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range tc.locks {
				if err := os.WriteFile(filepath.Join(root, name), []byte("lock"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := frontendUsesYarn(root); got != tc.yarn {
				t.Fatalf("frontendUsesYarn = %v, want %v", got, tc.yarn)
			}
		})
	}
}
