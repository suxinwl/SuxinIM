package wecom

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func TestConnectionTransferReencryptsWithoutBusinessData(t *testing.T) {
	ctx := setupDB(t)
	credentialTestKey(t)
	client := credentialTestClient(t, ctx)
	if err := client.SaveConfiguration(ctx, Settings{CorpID: "dummy-corp", AgentID: "100", IncludeSubdepartments: true}, CredentialUpdate{ContactSecret: "portable-contact-secret", AppSecret: "portable-app-secret"}); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x54}, 32)
	sealed, err := ExportConnection(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, []byte("portable-contact-secret")) || bytes.Contains(sealed, []byte("dummy-corp")) {
		t.Fatal("transfer exposed plaintext")
	}
	if err = RestoreConnection(ctx, key, sealed); err == nil {
		t.Fatal("restore overwrote existing connection")
	}
	for _, table := range []string{"wecom_settings", "wecom_credentials"} {
		if _, err = core.Model(table).Ctx(ctx).Where("1=1").Delete(); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WECOM_CREDENTIAL_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x76}, 32)))
	if err = RestoreConnection(ctx, key, sealed); err != nil {
		t.Fatal(err)
	}
	client = NewFromEnv()
	if err = client.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	if client.ContactSecret != "portable-contact-secret" || client.AppSecret != "portable-app-secret" {
		t.Fatal("portable configuration did not reencrypt for target key")
	}
	var employees int
	employees, err = core.Model("biz_hrm_employee").Ctx(ctx).Count()
	if err != nil || employees != 0 {
		t.Fatal("connection transfer imported employees")
	}
}
func TestSyncStartFloorAppliesToHistoricalAndNightlyRanges(t *testing.T) {
	t.Setenv("GOSUXIN_SYNC_START_AT", "2026-11-02T09:00:00+08:00")
	floor, _ := SyncFloor()
	end := floor.Add(time.Hour)
	for _, kind := range []string{"checkin", "approval", "leave", "out", "trip", "overtime"} {
		start, err := applySyncFloor(kind, end.Add(-30*24*time.Hour), end)
		if err != nil || !start.Equal(floor) {
			t.Fatal("historical request bypassed floor", kind, err)
		}
	}
	if _, err := applySyncFloor("checkin", floor.Add(-time.Hour), floor); err == nil {
		t.Fatal("request entirely before activation was accepted")
	}
	t.Setenv("GOSUXIN_SYNC_START_AT", "invalid")
	if _, err := SyncFloor(); err == nil {
		t.Fatal("invalid deployment floor accepted")
	}
}
