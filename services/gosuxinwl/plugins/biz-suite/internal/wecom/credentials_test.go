package wecom

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

func credentialTestKey(t *testing.T) string {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	t.Setenv("WECOM_CREDENTIAL_KEY", key)
	t.Setenv("SUXIN_PLUGIN_DATA", "")
	return key
}

func credentialTestClient(t *testing.T, ctx context.Context) *Client {
	t.Helper()
	c := &Client{CorpID: "dummy-corp", AgentID: "100", ContactSecret: "dummy-env-contact", AppSecret: "dummy-env-app", Secret: "dummy-legacy-app"}
	if err := c.LoadSettings(ctx); err != nil {
		t.Fatal(err)
	}
	return c
}

func credentialRowsSnapshot(t *testing.T, ctx context.Context) string {
	t.Helper()
	rows, err := core.Model("wecom_credentials").Ctx(ctx).OrderAsc("scope").All()
	if err != nil {
		t.Fatal(err)
	}
	settings, err := core.Model("wecom_settings").Ctx(ctx).All()
	if err != nil {
		t.Fatal(err)
	}
	return mustJSON(g.Map{"credentials": rows, "settings": settings})
}

func TestCredentialCipherAuthenticatesScopeAndRejectsCorruption(t *testing.T) {
	key := credentialTestKey(t)
	const secret = "dummy-contact-value-never-persist-plaintext"
	a, err := encryptCredential("contact", secret)
	if err != nil {
		t.Fatal(err)
	}
	b, err := encryptCredential("contact", secret)
	if err != nil || a == b || strings.Contains(a, secret) || !strings.HasPrefix(a, "v1:") {
		t.Fatal("ciphertext must be versioned, randomized and hide the plaintext", err)
	}
	if got, err := decryptCredential("contact", a); err != nil || got != secret {
		t.Fatal("credential roundtrip failed", err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(a, "v1:"))
	if err != nil || bytes.Contains(raw, []byte(secret)) {
		t.Fatal("invalid ciphertext encoding or plaintext in ciphertext")
	}
	raw[len(raw)-1] ^= 1
	for _, tc := range []struct{ scope, value string }{
		{"app", a}, {"contact", "v1:" + base64.StdEncoding.EncodeToString(raw)},
		{"contact", "v1:invalid-base64"}, {"contact", "v1:"}, {"contact", "v2:" + a},
	} {
		plain, err := decryptCredential(tc.scope, tc.value)
		if err == nil || plain != "" || strings.Contains(err.Error(), secret) {
			t.Fatal("invalid ciphertext or scope must fail closed without exposing plaintext")
		}
	}
	t.Setenv("WECOM_CREDENTIAL_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x43}, 32)))
	if plain, err := decryptCredential("contact", a); err == nil || plain != "" {
		t.Fatal("wrong master key accepted")
	}
	t.Setenv("WECOM_CREDENTIAL_KEY", "")
	if plain, err := decryptCredential("contact", a); err == nil || plain != "" {
		t.Fatal("missing master key accepted")
	}
	for _, invalid := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("too-short"))} {
		t.Setenv("WECOM_CREDENTIAL_KEY", invalid)
		if _, err := encryptCredential("contact", secret); err == nil {
			t.Fatal("invalid master key accepted")
		}
	}
	t.Setenv("WECOM_CREDENTIAL_KEY", key)
}

func TestCredentialPrivateKeyFileRoundtrip(t *testing.T) {
	t.Setenv("WECOM_CREDENTIAL_KEY", "")
	dir := filepath.Join(t.TempDir(), "private")
	t.Setenv("SUXIN_PLUGIN_DATA", dir)
	if _, err := credentialKey(false); err == nil {
		t.Fatal("missing private key must not be generated while reading")
	}
	key, err := credentialKey(true)
	if err != nil {
		// Do not skip DPAPI errors: deployment must prove the service account can
		// protect and recover its key, or explicitly configure an environment key.
		t.Fatalf("private key creation/DPAPI failed: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "wecom-credentials.key"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" && (!bytes.HasPrefix(raw, []byte("WDP1")) || bytes.Contains(raw, key)) {
		t.Fatal("Windows key file must contain a DPAPI-protected blob")
	}
	if runtime.GOOS != "windows" {
		stat, err := os.Stat(filepath.Join(dir, "wecom-credentials.key"))
		if err != nil || stat.Mode().Perm()&0077 != 0 {
			t.Fatal("private key must not be readable by other POSIX users")
		}
	}
	again, err := credentialKey(false)
	if err != nil || !bytes.Equal(key, again) {
		t.Fatal("private key could not be recovered", err)
	}
	sealed, err := encryptCredential("app", "dummy-private-key-roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := decryptCredential("app", sealed); err != nil || got != "dummy-private-key-roundtrip" {
		t.Fatal("private-file credential roundtrip failed", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wecom-credentials.key"), []byte("invalid-key-file"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := credentialKey(true); err == nil {
		t.Fatal("existing corrupt key must not be silently replaced")
	}
}

func TestCredentialConfigurationLifecycleAndRedaction(t *testing.T) {
	credentialTestKey(t)
	ctx := setupDB(t)
	c := credentialTestClient(t, ctx)
	s := Settings{CorpID: "dummy-corp", AgentID: "200", IncludeSubdepartments: true}
	c.tokens = map[string]cachedToken{"dummy-env-app": {Value: "dummy-token", Until: time.Now().Add(time.Hour)}}
	update := CredentialUpdate{ContactSecret: " dummy-saved-contact ", AppSecret: "dummy-saved-app"}
	if err := c.SaveConfiguration(ctx, s, update); err != nil {
		t.Fatal(err)
	}
	if c.ContactSecret != "dummy-saved-contact" || c.AppSecret != "dummy-saved-app" || len(c.tokens) != 0 {
		t.Fatal("saved credentials were not activated or token cache was retained")
	}
	rows, err := core.Model("wecom_credentials").All()
	if err != nil || len(rows) != 2 {
		t.Fatal("expected two encrypted credential rows", err)
	}
	for _, row := range rows {
		if len(row) != 3 || row["scope"] == nil || row["encrypted_secret"] == nil || row["updated_at"] == nil {
			t.Fatal("credential table must contain only scope/ciphertext/update time")
		}
		if !strings.HasPrefix(row["encrypted_secret"].String(), "v1:") {
			t.Fatal("credential table contains a non-ciphertext value")
		}
	}
	public := mustJSON(c.Settings()) + mustJSON(c.Status())
	stored := credentialRowsSnapshot(t, ctx)
	for _, secret := range []string{"dummy-saved-contact", "dummy-saved-app", "dummy-env-contact", "dummy-env-app", "dummy-legacy-app"} {
		if strings.Contains(public, secret) || strings.Contains(stored, secret) {
			t.Fatal("credential plaintext escaped into settings/status or database fields")
		}
	}
	restarted := credentialTestClient(t, ctx)
	if restarted.ContactSecret != c.ContactSecret || restarted.AppSecret != c.AppSecret || restarted.AgentID != "200" {
		t.Fatal("saved settings/credentials did not survive restart")
	}
	before := credentialRowsSnapshot(t, ctx)
	if err := restarted.SaveConfiguration(ctx, s, CredentialUpdate{ContactSecret: " \t ", AppSecret: ""}); err != nil {
		t.Fatal(err)
	}
	if restarted.ContactSecret != "dummy-saved-contact" || restarted.AppSecret != "dummy-saved-app" {
		t.Fatal("blank fields must preserve saved credentials")
	}
	// Compare encrypted rows separately: settings timestamps may advance.
	after, _ := core.Model("wecom_credentials").All()
	if mustJSON(rows) != mustJSON(after) || strings.Contains(before, "dummy-saved") {
		t.Fatal("blank fields unexpectedly rewrote credentials")
	}
	restarted.tokens = map[string]cachedToken{"dummy-saved-app": {Value: "dummy-token"}}
	if err := restarted.SaveConfiguration(ctx, s, CredentialUpdate{ClearContact: true, ClearApp: true}); err != nil {
		t.Fatal(err)
	}
	if restarted.ContactSecret != "dummy-env-contact" || restarted.AppSecret != "dummy-env-app" || restarted.savedContactSecret || restarted.savedAppSecret || len(restarted.tokens) != 0 || countRows(t, ctx, "wecom_credentials") != 0 {
		t.Fatal("explicit clear must remove saved values, invalidate tokens and restore environment credentials")
	}
	if reloaded := credentialTestClient(t, ctx); reloaded.ContactSecret != "dummy-env-contact" || reloaded.AppSecret != "dummy-env-app" {
		t.Fatal("cleared credentials returned after restart")
	}
}

func TestCredentialSaveTransactionRollbackAndBusyGuard(t *testing.T) {
	credentialTestKey(t)
	ctx := setupDB(t)
	c := credentialTestClient(t, ctx)
	s := Settings{CorpID: "dummy-corp", AgentID: "200"}
	if err := c.SaveConfiguration(ctx, s, CredentialUpdate{ContactSecret: "dummy-old-contact", AppSecret: "dummy-old-app"}); err != nil {
		t.Fatal(err)
	}
	before := credentialRowsSnapshot(t, ctx)
	if _, err := core.DB().Exec(ctx, `CREATE TRIGGER reject_app_credential BEFORE UPDATE ON wecom_credentials WHEN NEW.scope='app' BEGIN SELECT RAISE(ABORT, 'dummy-write-failure'); END`); err != nil {
		t.Fatal(err)
	}
	c.tokens = map[string]cachedToken{"dummy-old-app": {Value: "dummy-old-token"}}
	changed := Settings{CorpID: "dummy-corp", AgentID: "300"}
	if err := c.SaveConfiguration(ctx, changed, CredentialUpdate{ClearContact: true, AppSecret: "dummy-new-app"}); err == nil {
		t.Fatal("forced credential storage failure did not fail save")
	}
	if before != credentialRowsSnapshot(t, ctx) || c.ContactSecret != "dummy-old-contact" || c.AppSecret != "dummy-old-app" || c.AgentID != "200" || len(c.tokens) != 1 {
		t.Fatal("failed transaction partially changed settings, credentials or runtime state")
	}
	if !c.claim() {
		t.Fatal("could not claim busy state")
	}
	err := c.SaveConfiguration(ctx, changed, CredentialUpdate{ClearContact: true, ClearApp: true})
	c.release()
	if err == nil || before != credentialRowsSnapshot(t, ctx) {
		t.Fatal("busy guard allowed credential changes")
	}
	for _, update := range []CredentialUpdate{{ContactSecret: "dummy-new", ClearContact: true}, {AppSecret: "masked***"}, {ContactSecret: "embedded space"}, {AppSecret: strings.Repeat("a", 513)}} {
		if err := c.SaveConfiguration(ctx, changed, update); err == nil || before != credentialRowsSnapshot(t, ctx) {
			t.Fatal("invalid credential update changed stored configuration")
		}
	}
}

func TestCredentialLoadRejectsScopeSwapAndDoesNotPartiallyApply(t *testing.T) {
	credentialTestKey(t)
	ctx := setupDB(t)
	c := credentialTestClient(t, ctx)
	if err := c.SaveConfiguration(ctx, Settings{CorpID: "dummy-corp"}, CredentialUpdate{ContactSecret: "dummy-saved-contact", AppSecret: "dummy-saved-app"}); err != nil {
		t.Fatal(err)
	}
	contact, err := core.Model("wecom_credentials").Where("scope", "contact").Value("encrypted_secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := core.Model("wecom_credentials").Where("scope", "app").Update(g.Map{"encrypted_secret": contact.String()}); err != nil {
		t.Fatal(err)
	}
	restarted := &Client{CorpID: "dummy-corp", ContactSecret: "dummy-env-contact", AppSecret: "dummy-env-app"}
	if err := restarted.LoadSettings(ctx); err == nil {
		t.Fatal("scope-swapped ciphertext was accepted at startup")
	}
	if restarted.ContactSecret != "dummy-env-contact" || restarted.AppSecret != "dummy-env-app" || restarted.savedContactSecret || restarted.savedAppSecret {
		t.Fatal("failed startup partially applied decrypted credentials")
	}
}

func TestCredentialSaveRejectsChangedOrMissingMasterKey(t *testing.T) {
	for _, mode := range []string{"wrong", "missing"} {
		t.Run(mode, func(t *testing.T) {
			credentialTestKey(t)
			ctx := setupDB(t)
			c := credentialTestClient(t, ctx)
			s := Settings{CorpID: "dummy-corp", AgentID: "200"}
			if err := c.SaveConfiguration(ctx, s, CredentialUpdate{ContactSecret: "dummy-old-contact", AppSecret: "dummy-old-app"}); err != nil {
				t.Fatal(err)
			}
			before := credentialRowsSnapshot(t, ctx)
			keyDir := t.TempDir()
			if mode == "wrong" {
				t.Setenv("WECOM_CREDENTIAL_KEY", base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x55}, 32)))
			} else {
				t.Setenv("WECOM_CREDENTIAL_KEY", "")
				t.Setenv("SUXIN_PLUGIN_DATA", keyDir)
			}
			if err := c.SaveConfiguration(ctx, Settings{CorpID: "dummy-corp", AgentID: "300"}, CredentialUpdate{AppSecret: "dummy-new-app"}); err == nil {
				t.Fatal("changing or losing the master key must not produce mixed-key credential rows")
			}
			if before != credentialRowsSnapshot(t, ctx) || c.AppSecret != "dummy-old-app" || c.AgentID != "200" {
				t.Fatal("master key failure partially changed live or saved configuration")
			}
			if mode == "missing" {
				if _, err := os.Stat(filepath.Join(keyDir, "wecom-credentials.key")); !os.IsNotExist(err) {
					t.Fatal("missing master key was silently recreated for an existing credential store")
				}
			}
			restarted := &Client{CorpID: "dummy-corp", ContactSecret: "dummy-env-contact", AppSecret: "dummy-env-app"}
			if err := restarted.LoadSettings(ctx); err == nil {
				t.Fatal("unreadable saved credentials silently fell back to environment")
			}
		})
	}
}
