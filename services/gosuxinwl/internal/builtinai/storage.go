package builtinai

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/suxinwl/GoSuxin/internal/devtools/ai"
)

const (
	maxMigrationFileBytes  = 4 << 20
	maxMigrationTotalBytes = 64 << 20
	maxMigrationEntries    = 10000
)

type serviceState struct {
	once    sync.Once
	service *ai.Service
	err     error
}

var systemAI = &serviceState{}

// DefaultService creates the host-owned service once. The caller must quiesce
// the legacy composite worker before its first invocation to prevent concurrent
// writes while its encrypted configuration and developer tokens are copied.
func DefaultService() (*ai.Service, error) {
	systemAI.once.Do(func() {
		base := os.Getenv("SUXIN_SYSTEM_DATA")
		if base == "" {
			base = filepath.Join("storage", "system")
		}
		target, err := filepath.Abs(filepath.Join(base, "ai"))
		if err != nil {
			systemAI.err = err
			return
		}
		source, err := filepath.Abs(filepath.Join("storage", "plugins", "devtools", "data", "ai"))
		if err != nil {
			systemAI.err = err
			return
		}
		if err := MigrateLegacyAI(source, target); err != nil {
			systemAI.err = err
			return
		}
		systemAI.service, systemAI.err = ai.NewService(target)
	})
	return systemAI.service, systemAI.err
}

func sameMigrationPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func nestedMigrationPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		a, b = strings.ToLower(a), strings.ToLower(b)
	}
	rel, err := filepath.Rel(a, b)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// rejectMigrationLinks checks existing ancestors as well as the final path so a
// source or target cannot escape through a directory symlink or junction.
func rejectMigrationLinks(absolute string) error {
	for current := absolute; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return errors.New("AI 迁移路径含符号链接或非目录节点")
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
	}
}

// migrationTarget returns whether a valid existing master-key record makes the
// destination authoritative. A nonempty directory without a key is not safe to
// merge: generating a new key could orphan its encrypted configuration.
func migrationTarget(target string) (authoritative, empty bool, err error) {
	info, err := os.Lstat(target)
	if os.IsNotExist(err) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, false, errors.New("AI 目标必须为普通目录")
	}
	key, err := os.Lstat(filepath.Join(target, "master.key"))
	if err == nil {
		if !key.Mode().IsRegular() || key.Mode()&os.ModeSymlink != 0 || key.Size() > maxMigrationFileBytes {
			return false, false, errors.New("AI 目标密钥文件无效")
		}
		return true, false, nil
	}
	if !os.IsNotExist(err) {
		return false, false, err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return false, false, err
	}
	if len(entries) != 0 {
		return false, false, errors.New("AI 目标目录已有配置但缺少密钥，拒绝覆盖或合并")
	}
	return false, true, nil
}

func removeMigrationStage(parent, stage string) error {
	if err := rejectMigrationLinks(parent); err != nil {
		return err
	}
	abs, err := filepath.Abs(stage)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(parent, abs)
	if err != nil || filepath.Dir(rel) != "." || !strings.HasPrefix(filepath.Base(rel), ".ai-migrate-") {
		return errors.New("AI 临时目录清理路径不在指定目标父目录内")
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("AI 临时目录被替换，拒绝递归清理")
	}
	return os.RemoveAll(abs)
}

func copyMigrationFile(source, target string, expected os.FileInfo) error {
	if !expected.Mode().IsRegular() || expected.Mode()&os.ModeSymlink != 0 || expected.Size() > maxMigrationFileBytes {
		return errors.New("AI 源文件为特殊文件、链接或超过大小限制")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	actual, err := input.Stat()
	if err != nil {
		return err
	}
	if !actual.Mode().IsRegular() || !os.SameFile(expected, actual) || actual.Size() != expected.Size() {
		return errors.New("AI 源文件在复制前发生变化")
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(output, io.LimitReader(input, maxMigrationFileBytes+1))
	if copyErr == nil && (n != expected.Size() || n > maxMigrationFileBytes) {
		copyErr = errors.New("AI 源文件在复制中发生变化或超出限制")
	}
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	current, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, current) || current.Size() != expected.Size() || !current.ModTime().Equal(expected.ModTime()) {
		return errors.New("AI 源文件在复制期间发生变化")
	}
	return nil
}

// MigrateLegacyAI copies the old complete AI directory without changing it.
// Existing destination configuration is authoritative. The caller must stop
// legacy writes first; migration publishes a fully copied, private directory
// with one rename on the destination filesystem.
func MigrateLegacyAI(source, target string) (err error) {
	if strings.TrimSpace(source) == "" || strings.TrimSpace(target) == "" {
		return errors.New("AI 迁移源与目标路径不能为空")
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	source, target = filepath.Clean(source), filepath.Clean(target)
	if sameMigrationPath(source, target) || nestedMigrationPath(source, target) || nestedMigrationPath(target, source) {
		return errors.New("AI 迁移源与目标不能相同或互为父子目录")
	}
	if err := rejectMigrationLinks(target); err != nil {
		return err
	}
	authoritative, _, err := migrationTarget(target)
	if err != nil || authoritative {
		return err
	}
	if err := rejectMigrationLinks(source); err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("AI 迁移源必须为目录")
	}
	master, err := os.Lstat(filepath.Join(source, "master.key"))
	if err != nil {
		return errors.New("AI 旧数据缺少完整的加密密钥，拒绝迁移")
	}
	if !master.Mode().IsRegular() || master.Mode()&os.ModeSymlink != 0 || master.Size() != 32 {
		return errors.New("AI 旧加密密钥文件无效")
	}
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	if err := rejectMigrationLinks(parent); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, ".ai-migrate-")
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := removeMigrationStage(parent, stage); err == nil && cleanupErr != nil {
			err = cleanupErr
		}
	}()
	if err := os.Chmod(stage, 0700); err != nil {
		return err
	}
	var total int64
	entries := 0
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entries++
		if entries > maxMigrationEntries {
			return errors.New("AI 旧数据文件数量超过限制")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("AI 旧数据包含符号链接，拒绝迁移")
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) > 8 || (parts[0] != "master.key" && parts[0] != "config" && parts[0] != "tokens") {
			return errors.New("AI 旧数据包含未知目录或过深路径")
		}
		destination := filepath.Join(stage, rel)
		if entry.IsDir() {
			if parts[0] == "master.key" {
				return errors.New("AI 密钥节点不能为目录")
			}
			return os.Mkdir(destination, 0700)
		}
		if !info.Mode().IsRegular() || info.Size() > maxMigrationFileBytes {
			return errors.New("AI 旧数据包含特殊文件或过大文件")
		}
		if parts[0] != "master.key" && filepath.Ext(rel) != ".json" {
			return errors.New("AI 配置及令牌目录包含非 JSON 数据文件")
		}
		total += info.Size()
		if total > maxMigrationTotalBytes {
			return errors.New("AI 旧数据总大小超过限制")
		}
		return copyMigrationFile(path, destination, info)
	})
	if err != nil {
		return err
	}
	if err := rejectMigrationLinks(target); err != nil {
		return err
	}
	authoritative, empty, err := migrationTarget(target)
	if err != nil || authoritative {
		return err
	}
	if empty {
		// Windows cannot rename onto an existing empty directory. Remove only the
		// just-checked empty node; a concurrent nonempty target will fail safely.
		if err := os.Remove(target); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, target); err != nil {
		if empty {
			if restoreErr := os.Mkdir(target, 0700); restoreErr != nil && !os.IsExist(restoreErr) {
				return fmt.Errorf("AI 数据发布失败，空目标目录恢复也失败：%w", err)
			}
		}
		return err
	}
	return nil
}
