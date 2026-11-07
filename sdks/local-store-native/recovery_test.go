package localstore

import (
	"bytes"
	"testing"
)

func TestRecoveryPasswordWrapAndIdentity(t *testing.T) {
	master := bytes.Repeat([]byte{0x17}, 32)
	account := "synthetic-recovery"
	password := "synthetic-password-123"
	packageBytes, err := ExportRecovery(master, account, password)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := ImportRecovery(packageBytes, account, password)
	if err != nil || !bytes.Equal(restored, master) {
		t.Fatal("recovery roundtrip failed", err)
	}
	for _, test := range []struct{ account, password string }{{account, "wrong-password-123"}, {"other-account", password}} {
		if _, err = ImportRecovery(packageBytes, test.account, test.password); err == nil {
			t.Fatal("wrong recovery identity accepted")
		}
	}
	if _, err = ExportRecovery(master, account, "short"); err == nil {
		t.Fatal("short recovery password accepted")
	}
	if bytes.Contains(packageBytes, master) || bytes.Contains(packageBytes, []byte(password)) {
		t.Fatal("recovery contains plaintext secret")
	}
}
