//go:build android || ios

package open_im_sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	nativestore "puty.local/nativestore"
	"strings"
	"sync"
	"syscall"
)

// Native-only binding. The Flutter plugin never exposes a master-key argument
// on its MethodChannel; its OS broker calls this after online actor validation.
func ConfigureNativeLocalStore(account, directory string, master []byte, authorizedUntil int64) bool {
	return nativestore.Configure(account, directory, master, authorizedUntil) == nil
}
func LockNativeLocalStore(account string) { nativestore.Lock(account) }
func ConfigureNativeLegacyQuarantine(account, directory string, master []byte) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.ConfigureLegacyQuarantine(directory, master) == nil
}
func FinalizeNativeLegacyFiles(account string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.QuarantineLegacyOwnedFiles() == nil
}
func SetNativeLegacyRoots(account, encoded string) bool {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return false
	}
	var roots []string
	if json.Unmarshal([]byte(encoded), &roots) != nil {
		return false
	}
	return s.SetLegacyRoots(roots) == nil
}

// Native integration tools may open an already authorized/configured store.
// There is deliberately no matching Dart MethodChannel or default key path.
func OpenConfiguredNativeWorkingStore(account string) bool {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return false
	}
	_, err = s.OpenWorkingDatabase()
	return err == nil
}
func RecoverNativeStoreRoot(directory string) bool {
	return nativestore.RecoverPromotion(directory) == nil
}
func ExportNativeBackup(account, passphrase string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	path, _ := s.ExportBackup(passphrase)
	return path
}

// NativeRestore is retained by the OS broker, never serialized to Dart. The
// recovered master is wrapped with the OS key before the atomic promotion.
type NativeRestore struct{ prepared *nativestore.PreparedRestore }

func PrepareNativeRestore(account, path, passphrase string) *NativeRestore {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	prepared, err := s.PrepareRestore(path, passphrase)
	if err != nil {
		return nil
	}
	return &NativeRestore{prepared}
}
func (r *NativeRestore) Master() []byte {
	if r == nil || r.prepared == nil {
		return nil
	}
	return r.prepared.MasterKey()
}
func (r *NativeRestore) Commit(wrapped []byte) bool {
	nativeRestoreFence.Lock()
	defer nativeRestoreFence.Unlock()
	if UserForSDK != nil && UserForSDK.GetLoginStatus(context.Background()) != LogoutStatus {
		return false
	}
	if r == nil || r.prepared == nil {
		return false
	}
	return r.prepared.Commit(wrapped) == nil
}

// Long verification stays outside the OS broker transition so online renewal
// can continue. Neither this phase nor a late old actor can promote a root.
func (r *NativeRestore) PrepareCommit(wrapped []byte) bool {
	nativeRestoreFence.Lock()
	defer nativeRestoreFence.Unlock()
	if UserForSDK != nil && UserForSDK.GetLoginStatus(context.Background()) != LogoutStatus {
		return false
	}
	return r != nil && r.prepared != nil && r.prepared.PrepareCommit(wrapped) == nil
}
func (r *NativeRestore) Abort() {
	if r != nil && r.prepared != nil {
		r.prepared.Abort()
		r.prepared = nil
	}
}
func FlushNativeLocalArchive(account string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.Flush(1000) == nil
}
func GetNativeLocalArchive(account, conversation string, before int64, limit int) string {
	return GetNativeLocalArchivePage(account, conversation, before, "", limit)
}
func GetNativeLocalArchivePage(account, conversation string, before int64, beforeID string, limit int) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return `{"error":"LOCKED"}`
	}
	page, err := s.PageCursor(conversation, before, beforeID, limit)
	if err != nil {
		return `{"error":"ARCHIVE_UNAVAILABLE"}`
	}
	data, _ := json.Marshal(map[string]any{"items": page})
	return string(data)
}
func ClearNativeLocalRange(account, conversation string, from, to int64) bool {
	if conversation == "" || strings.ContainsAny(conversation, "\x00/\\\"'`") {
		return false
	}
	s, err := nativestore.Lookup(account)
	if err != nil {
		return false
	}
	return s.ClearRange("chat_logs_"+conversation, conversation, from, to) == nil
}
func ImportNativeLocalAttachment(account, path, conversation string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	item, err := s.ImportLocalAttachment(path, conversation)
	if err != nil {
		return ""
	}
	data, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	return string(data)
}
func PreviewNativeClearRange(account, conversation string, from, to int64) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	preview, err := s.PreviewClear("chat_logs_"+conversation, conversation, from, to)
	if err != nil {
		return ""
	}
	data, err := json.Marshal(preview)
	if err != nil {
		return ""
	}
	return string(data)
}
func PreviewNativeClearScopes(account, encodedIDs string, from, toExclusive int64) string {
	var ids []string
	if json.Unmarshal([]byte(encodedIDs), &ids) != nil {
		return ""
	}
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	result, err := s.PreviewClearScopes(ids, from, toExclusive)
	if err != nil {
		return ""
	}
	data, err := json.Marshal(result)
	if err != nil {
		return ""
	}
	return string(data)
}
func ClearNativeLocalScopes(account, encodedIDs string, from, toExclusive int64) bool {
	var ids []string
	if json.Unmarshal([]byte(encodedIDs), &ids) != nil {
		return false
	}
	s, err := nativestore.Lookup(account)
	return err == nil && s.ClearScopes(ids, from, toExclusive) == nil
}
func NativeLocalStoreStatus(account string) string {
	_, err := nativestore.Lookup(account)
	return fmt.Sprintf(`{"unlocked":%t,"format":"PUTY-LOCAL-STORE-1","workingDatabase":"SQLCipher"}`, err == nil)
}
func BeginNativeAttachment(account, task, objectID, sourceName, conversation, message string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	path, err := s.BeginAttachment(task, objectID, sourceName, conversation, message)
	if err != nil {
		return ""
	}
	return path
}
func WriteNativeAttachment(account, task string, chunk []byte) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.WriteAttachment(task, chunk) == nil
}
func FinishNativeAttachment(account, task string, expectedSize int64) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.FinishAttachment(task, expectedSize) == nil
}
func AbortNativeAttachment(account, task string) {
	if s, err := nativestore.Lookup(account); err == nil {
		s.AbortAttachment(task)
	}
}
func ReadNativeAttachment(account, task string, maxBytes int64) []byte {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	data, err := s.ReadAttachment(task, maxBytes)
	if err != nil {
		return nil
	}
	return data
}
func NativeAttachmentComplete(account, task string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.AttachmentComplete(task)
}
func NativeAttachmentOffset(account, task string) int64 {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return -1
	}
	return s.AttachmentOffset(task)
}
func NativeAttachmentETag(account, task string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	return s.AttachmentETag(task)
}
func SetNativeAttachmentETag(account, task, etag string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.SetAttachmentETag(task, etag) == nil
}
func ResetNativeAttachment(account, task string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.ResetAttachment(task) == nil
}

func BeginNativeScratch(account, name string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	id, _ := s.BeginScratch(name)
	return id
}
func NativeScratchSize(account, id string) int64 {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return -1
	}
	size, err := s.ScratchSize(id)
	if err != nil {
		return -1
	}
	return size
}
func ReadNativeScratch(account, id string, offset int64, count int) []byte {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	data, _ := s.ReadScratch(id, offset, count)
	return data
}
func WriteNativeScratch(account, id string, offset int64, data []byte) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.WriteScratch(id, offset, data) == nil
}
func CloseNativeScratch(account, id string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.CloseScratch(id) == nil
}
func StageNativeScratch(account, id string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	path, _ := s.StageScratch(id)
	return path
}
func StageNativeSource(account, path string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	result, _ := s.PrepareOutgoing(path)
	return result
}
func ReadNativeSource(account, path string, max int64) []byte {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	data, _ := s.ReadOutgoing(path, max)
	return data
}
func StageNativeBytes(account, name string, data []byte) string {
	defer clear(data)
	if len(data) > 32<<20 {
		return ""
	}
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	path, _ := s.StageOutgoing(bytes.NewReader(data), name)
	return path
}
func NativeSourceInfo(account, path string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	name, size, err := s.SourceInfo(path)
	if err != nil {
		return ""
	}
	data, _ := json.Marshal(map[string]any{"name": name, "size": size})
	return string(data)
}
func DiscardNativeSource(account, path string) bool {
	s, err := nativestore.Lookup(account)
	return err == nil && s.DiscardUnboundSource(path) == nil
}
func ReadNativeSourceRange(account, path string, offset int64, count int) []byte {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	data, _ := s.ReadOutgoingRange(path, offset, count)
	return data
}
func ReadNativeAttachmentRange(account, task string, offset int64, count int) []byte {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return nil
	}
	data, _ := s.ReadAttachmentRange(task, offset, count)
	return data
}
func ListNativeAttachments(account, conversation, cursor string, limit int) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	page, err := s.AttachmentInventory(conversation, cursor, limit)
	if err != nil {
		return ""
	}
	data, _ := json.Marshal(page)
	return string(data)
}
func NativeAttachmentInfo(account, task string) string {
	s, err := nativestore.Lookup(account)
	if err != nil {
		return ""
	}
	info, err := s.AttachmentInfo(task)
	if err != nil {
		return ""
	}
	data, _ := json.Marshal(info)
	return string(data)
}

// Native-only descriptor handoff. Duplicate it before handing ownership to Go;
// SAF providers that cannot seek fail closed without a plaintext temp copy.
func PrepareNativeRestoreDescriptor(account string, descriptor int, passphrase string) *NativeRestore {
	s, err := nativestore.Lookup(account)
	if err != nil || descriptor < 0 {
		return nil
	}
	fd, err := syscall.Dup(descriptor)
	if err != nil {
		return nil
	}
	file := os.NewFile(uintptr(fd), "protected-backup-input")
	defer file.Close()
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return nil
	}
	prepared, err := s.PrepareRestoreReader(file, size, passphrase)
	if err != nil {
		return nil
	}
	return &NativeRestore{prepared}
}

var nativeRestoreFence sync.RWMutex

func nativeLoginFence() func() { nativeRestoreFence.RLock(); return nativeRestoreFence.RUnlock }
