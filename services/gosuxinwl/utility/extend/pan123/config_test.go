package pan123

import "testing"

func TestMergePreservesSecretsAndRejectsChangedAccount(t *testing.T) {
	old := Config{ClientID: "client", ClientSecret: "saved-secret", CDNKey: "saved-cdn", UID: 7}
	p, e := Merge(old, Config{ClientID: "client"})
	if e != nil {
		t.Fatal(e)
	}
	if p.ClientSecret != "saved-secret" || p.CDNKey != "saved-cdn" || !p.URLAuthEnabled() {
		t.Fatal("blank fields did not preserve configured secrets")
	}
	if _, e = Merge(old, Config{ClientID: "another"}); e == nil {
		t.Fatal("reused old secret with a different clientID")
	}
	off := false
	p, e = Merge(old, Config{ClientID: "another", ClientSecret: "new-secret", URLAuth: &off})
	if e != nil || p.CDNKey != "" || p.URLAuthEnabled() {
		t.Fatal("unsigned new account incorrectly inherited CDN key")
	}
}
