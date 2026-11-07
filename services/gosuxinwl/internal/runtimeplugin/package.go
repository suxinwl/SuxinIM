// Package runtimeplugin installs and supervises precompiled, trusted plugins.
package runtimeplugin

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const Format = "suxin-runtime-v1"
const MaxArchive = 160 << 20
const maxExpanded = 320 << 20

func StageUpload(filename string) (string, error) {
	if err := os.MkdirAll("storage/plugins/uploads", 0700); err != nil {
		return "", err
	}
	token := "runtime-" + nonce()
	err := os.Rename(filename, filepath.Join("storage/plugins/uploads", token+".zip"))
	return token, err
}

type Manifest struct {
	Format   string            `json:"format"`
	Name     string            `json:"name"`
	Title    string            `json:"title"`
	Version  string            `json:"version"`
	Protocol int               `json:"protocol"`
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Files    map[string]string `json:"files"`
}

func safePath(p string) bool {
	if p == "" || path.Clean(p) != p || strings.ContainsAny(p, "\\:") || strings.HasPrefix(p, "/") {
		return false
	}
	for _, s := range strings.Split(p, "/") {
		if s == ".." || strings.TrimRight(s, ". ") != s || strings.HasPrefix(s, ".") {
			return false
		}
		stem := strings.ToUpper(strings.SplitN(s, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9' {
			return false
		}
	}
	return strings.HasPrefix(p, "bin/") || p == "README.md" || p == "LICENSE.txt" || strings.HasPrefix(p, "admin/") || strings.HasPrefix(p, "public/")
}

// Inspect verifies every byte before an executable can be installed. Hashes
// detect damage, not publisher identity: only trusted administrators may upload.
func Inspect(filename string) (Manifest, error) {
	m, _, err := unpack(filename, "")
	return m, err
}

func unpack(filename, dest string) (Manifest, string, error) {
	var m Manifest
	info, err := os.Stat(filename)
	if err != nil {
		return m, "", err
	}
	if info.Size() > MaxArchive {
		return m, "", errors.New("插件 ZIP 超过 160 MB")
	}
	z, err := zip.OpenReader(filename)
	if err != nil {
		return m, "", err
	}
	defer z.Close()
	if len(z.File) > 3000 {
		return m, "", errors.New("插件文件数量超限")
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 || (f.Name != "plugin.json" && !safePath(f.Name)) {
			return m, "", fmt.Errorf("不允许的插件路径: %s", f.Name)
		}
		key := strings.ToLower(f.Name)
		if entries[key] != nil {
			return m, "", errors.New("插件包含重复路径")
		}
		entries[key] = f
		if f.UncompressedSize64 > MaxArchive {
			return m, "", errors.New("插件单文件过大")
		}
		total += f.UncompressedSize64
		if total > maxExpanded {
			return m, "", errors.New("插件解压大小超限")
		}
	}
	mf := entries["plugin.json"]
	if mf == nil || mf.Name != "plugin.json" || mf.UncompressedSize64 > 1<<20 {
		return m, "", errors.New("缺少插件清单")
	}
	r, err := mf.Open()
	if err != nil {
		return m, "", err
	}
	b, err := io.ReadAll(io.LimitReader(r, 1<<20+1))
	r.Close()
	if err != nil {
		return m, "", err
	}
	if err = json.Unmarshal(b, &m); err != nil {
		return m, "", err
	}
	if m.Format != Format || !pluginNamePattern.MatchString(m.Name) || m.Version == "" || m.Protocol != 1 {
		return m, "", errors.New("需要 Suxin V1.0.0 运行插件包；源码 ZIP 不能免编译安装")
	}
	if m.OS != runtime.GOOS || m.Arch != runtime.GOARCH {
		return m, "", fmt.Errorf("插件平台 %s/%s 与当前 %s/%s 不匹配", m.OS, m.Arch, runtime.GOOS, runtime.GOARCH)
	}
	exe := "bin/" + m.Name
	if m.OS == "windows" {
		exe += ".exe"
	}
	if m.Files[exe] == "" || len(m.Files) != len(entries)-1 {
		return m, "", errors.New("插件文件不完整")
	}
	for name, want := range m.Files {
		f := entries[strings.ToLower(name)]
		if !safePath(name) || f == nil || f.Name != name {
			return m, "", errors.New("清单路径不匹配")
		}
		src, e := f.Open()
		if e != nil {
			return m, "", e
		}
		h := sha256.New()
		var out *os.File
		var w io.Writer = h
		if dest != "" {
			target := filepath.Join(dest, filepath.FromSlash(name))
			if e = os.MkdirAll(filepath.Dir(target), 0700); e == nil {
				out, e = os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			}
			if e != nil {
				src.Close()
				return m, "", e
			}
			w = io.MultiWriter(h, out)
		}
		n, e := io.Copy(w, io.LimitReader(src, MaxArchive+1))
		src.Close()
		if out != nil {
			ce := out.Close()
			if e == nil {
				e = ce
			}
		}
		if e != nil {
			return m, "", e
		}
		if n > MaxArchive || hex.EncodeToString(h.Sum(nil)) != want {
			return m, "", fmt.Errorf("插件校验失败: %s", name)
		}
	}
	digest := sha256.Sum256(b)
	if dest != "" {
		if err = os.WriteFile(filepath.Join(dest, "plugin.json"), b, 0600); err != nil {
			return m, "", err
		}
		if err = os.Chmod(filepath.Join(dest, filepath.FromSlash(exe)), 0700); err != nil {
			return m, "", err
		}
	}
	return m, hex.EncodeToString(digest[:]), nil
}
