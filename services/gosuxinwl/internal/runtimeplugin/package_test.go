package runtimeplugin

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func fixture(t *testing.T, mutate func(*Manifest, map[string][]byte)) string {
	t.Helper()
	exe := "bin/sampleplugin"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	files := map[string][]byte{exe: []byte("test executable"), "admin/index.html": []byte("admin")}
	m := Manifest{Format: Format, Name: "sampleplugin", Version: "1.0.0", Protocol: 1, OS: runtime.GOOS, Arch: runtime.GOARCH, Files: map[string]string{}}
	for name, data := range files {
		sum := sha256.Sum256(data)
		m.Files[name] = hex.EncodeToString(sum[:])
	}
	if mutate != nil {
		mutate(&m, files)
	}
	file := filepath.Join(t.TempDir(), "plugin.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(out)
	manifest, _ := json.Marshal(m)
	w, _ := z.Create("plugin.json")
	_, _ = w.Write(manifest)
	for name, data := range files {
		w, err = z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(data)
	}
	if err = z.Close(); err != nil {
		t.Fatal(err)
	}
	_ = out.Close()
	return file
}

func TestPackageValidation(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Manifest, map[string][]byte)
		valid  bool
	}{
		{"valid", nil, true},
		{"wrong platform", func(m *Manifest, _ map[string][]byte) { m.Arch = "wrong" }, false},
		{"source package", func(m *Manifest, _ map[string][]byte) { m.Format = "suxin-source-v1" }, false},
		{"corrupt contents", func(_ *Manifest, f map[string][]byte) { f["admin/index.html"] = []byte("changed") }, false},
		{"traversal", func(_ *Manifest, f map[string][]byte) { f["admin/../../outside"] = []byte("escape") }, false},
		{"case collision", func(_ *Manifest, f map[string][]byte) { f["admin/INDEX.html"] = []byte("duplicate") }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Inspect(fixture(t, tc.change))
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestSafePath(t *testing.T) {
	for _, p := range []string{"../bad", "/absolute", "bin/../bad", "admin/dir./file", "admin/x:stream", "admin\\x", "admin/.git/x", "admin/../config.yaml", "admin/CON.txt", "admin/LPT1"} {
		if safePath(p) {
			t.Fatal(p)
		}
	}
}
