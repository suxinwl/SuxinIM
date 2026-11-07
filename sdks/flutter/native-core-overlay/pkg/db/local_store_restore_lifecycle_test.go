//go:build android || ios

package db

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openimsdk/openim-sdk-core/v3/pkg/db/model_struct"
	nativestore "puty.local/nativestore"
)

func nativeSDKRestoreFixture(t *testing.T, preopen bool) (*nativestore.Store, *DataBase, *sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	account := strings.ReplaceAll(t.Name(), "/", "_")
	root := filepath.Join(t.TempDir(), "vault")
	master := bytes.Repeat([]byte{0x71}, 32)
	if err := nativestore.Configure(account, root, master, time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { nativestore.Lock(account); clear(master) })
	stop, renewed := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(renewed)
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if _, err := nativestore.Lookup(account); err == nil {
					_ = nativestore.Configure(account, root, master, time.Now().Add(8*time.Second).UnixMilli())
				}
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-renewed })
	store, err := nativestore.Lookup(account)
	if err != nil {
		t.Fatal(err)
	}
	legacyRoot := t.TempDir()
	deviceKey := bytes.Repeat([]byte{0x29}, 32)
	defer clear(deviceKey)
	if err = store.ConfigureLegacyQuarantine(t.TempDir(), deviceKey); err != nil {
		t.Fatal(err)
	}
	if err = store.SetLegacyRoots([]string{legacyRoot}); err != nil {
		t.Fatal(err)
	}
	var preopened *sql.DB
	if preopen {
		preopened, err = store.OpenWorkingDatabase()
		if err != nil {
			t.Fatal(err)
		}
	}
	sdk, err := NewDataBase(ctx, account, legacyRoot, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sdk.Close(ctx) })
	sdkSQL, err := sdk.conn.DB()
	if err != nil {
		t.Fatal(err)
	}
	if preopen && sdkSQL != preopened {
		t.Fatal("SDK did not adopt the broker pre-opened vault database")
	}
	return store, sdk, sdkSQL, root
}

func TestNativeSDKPreopenAndRepeatedSDKCloseReuseHandle(t *testing.T) {
	store, sdk, handle, _ := nativeSDKRestoreFixture(t, true)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := sdk.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if err := handle.Ping(); err != nil {
			t.Fatal("SDK-only close closed the broker's working database", err)
		}
		next, err := NewDataBase(ctx, sdk.loginUserID, sdk.dbDir, 0)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = next.Close(ctx) })
		nextSQL, err := next.conn.DB()
		if err != nil || nextSQL != handle {
			t.Fatal("SDK close/reopen replaced its native owner handle", err)
		}
		sdk = next
	}
	nativestore.Lock(sdk.loginUserID)
	if handle.Ping() == nil {
		t.Fatal("NativeLock retained a connection after repeated SDK reopen")
	}
	if _, err := store.OpenWorkingDatabase(); err == nil {
		t.Fatal("old native owner reopened after final lock")
	}
}

func TestNativeSDKCloseWaitsForRealWorkerFinalFlush(t *testing.T) {
	_, sdk, handle, _ := nativeSDKRestoreFixture(t, false)
	// The real SQLCipher connection is occupied while the actual archive worker
	// starts its final Flush. No fake worker, SQL callback or SDK state is used.
	handle.SetMaxOpenConns(1)
	held, err := handle.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	stop, done := sdk.archiveStop, sdk.archiveDone
	returned := make(chan error, 1)
	go func() { returned <- sdk.Close(context.Background()) }()
	select {
	case <-stop:
	case <-time.After(time.Second):
		t.Fatal("SDK Close did not signal its actual archive worker")
	}
	select {
	case <-done:
		t.Fatal("worker completion was signalled before final SQL flush could run")
	case err := <-returned:
		t.Fatal("SDK Close returned with its final flush still blocked", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = held.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SDK Close did not finish after the real SQL connection was released")
	}
	select {
	case <-done:
	default:
		t.Fatal("SDK Close completed before archiveDone")
	}
	if err = handle.Ping(); err != nil {
		t.Fatal("SDK-only close shut the native owner handle after draining", err)
	}
}

// This exercises the actual SDK DataBase.Close implementation, not a substitute
// store.Close call. Prepare occurs while the SDK is live, exactly as the UI does;
// the original bug fails during PrepareCommit's second privacy-state merge.
func TestNativeSDKDatabaseCloseDrainsArchiveAndAllowsRestoreCommit(t *testing.T) {
	store, sdk, sdkSQL, root := nativeSDKRestoreFixture(t, false)
	ctx := context.Background()
	conversation := "si_synthetic_sdk_restore"
	if err := sdk.initChatLog(ctx, conversation); err != nil {
		t.Fatal(err)
	}
	if err := sdk.InsertMessage(ctx, conversation, &model_struct.LocalChatLog{ClientMsgID: "synthetic-sdk-record", Content: "SYNTHETIC_SDK_RESTORE_BODY", ContentType: 101, SendTime: time.Now().UnixMilli(), Seq: 1, Status: 2}); err != nil {
		t.Fatal(err)
	}
	store.MarkViewsReady(true)
	backup, err := store.ExportBackup("synthetic core restore password")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.PrepareRestore(backup, "synthetic core restore password")
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Abort()
	restoredKey := prepared.MasterKey()
	defer clear(restoredKey)
	// A clear committed after the preview must still win over the older backup.
	if err = store.DeleteLocalMessage("chat_logs_"+conversation, conversation, "synthetic-sdk-record", time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	done := sdk.archiveDone
	if done == nil {
		t.Fatal("native SDK archive worker has no completion fence")
	}
	if err = sdk.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("SDK Close returned before the native archive worker drained")
	}
	envelope := bytes.Repeat([]byte{0x42}, 64)
	if err = prepared.PrepareCommit(envelope); err != nil {
		t.Fatal("actual SDK Close left PrepareCommit unable to merge current privacy state", err)
	}
	if err = prepared.Commit(envelope); err != nil {
		t.Fatal(err)
	}
	nativestore.Lock(sdk.loginUserID)
	if sdkSQL.Ping() == nil {
		t.Fatal("final native lock left the former SDK handle open")
	}
	if err = nativestore.Configure(sdk.loginUserID, root, restoredKey, time.Now().Add(8*time.Second).UnixMilli()); err != nil {
		t.Fatal(err)
	}
	restored, err := nativestore.Lookup(sdk.loginUserID)
	if err != nil {
		t.Fatal(err)
	}
	restoredDB, err := restored.OpenWorkingDatabase()
	if err != nil {
		t.Fatal(err)
	}
	var deleted int
	if err = restoredDB.QueryRow(`SELECT COUNT(*) FROM puty_local_deleted WHERE conversation=? AND message_id='synthetic-sdk-record'`, conversation).Scan(&deleted); err != nil || deleted != 1 {
		t.Fatal("actual SDK-close restore lost the post-preview exact delete", err)
	}
	if _, err = restored.Page(conversation, time.Now().Add(time.Hour).UnixMilli(), 10); err == nil {
		t.Fatal("restored history became readable before fresh SDK sync")
	}
	restored.MarkViewsReady(true)
	page, err := restored.Page(conversation, time.Now().Add(time.Hour).UnixMilli(), 10)
	if err != nil || len(page) != 0 {
		t.Fatal("older backup resurrected the record cleared after restore preview", err)
	}
}
