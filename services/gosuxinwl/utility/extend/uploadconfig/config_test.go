package uploadconfig

import (
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

func TestSavePreservesOtherProvidersAndEscapesSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.yaml")
	os.WriteFile(path, []byte("# upload settings\nType: local\nlocal:\n  LBaseUrl: https://example.com\nalioss:\n  ASecret: existing\n"), 0600)
	secret := "colon: hash# quote\" newline\n中文"
	if e := Save(path, map[string]interface{}{"Type": "pan123"}, "pan123", map[string]interface{}{"clientSecret": secret, "parentId": 0}); e != nil {
		t.Fatal(e)
	}
	if e := Save(path, map[string]interface{}{"MaxBodySize": 600}, "pan123", map[string]interface{}{"urlAuth": true}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	var got map[string]interface{}
	if e := yaml.Unmarshal(b, &got); e != nil {
		t.Fatal(e)
	}
	if got["pan123"].(map[string]interface{})["clientSecret"] != secret {
		t.Fatal("secret changed")
	}
	if got["alioss"].(map[string]interface{})["ASecret"] != "existing" || got["local"].(map[string]interface{})["LBaseUrl"] != "https://example.com" {
		t.Fatal("other provider changed")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary config leaked")
	}
}
func TestInvalidYamlLeavesOriginalIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upload.yaml")
	original := []byte("[broken")
	os.WriteFile(path, original, 0600)
	if e := Save(path, map[string]interface{}{"Type": "pan123"}, "pan123", nil); e == nil {
		t.Fatal("expected invalid YAML error")
	}
	got, _ := os.ReadFile(path)
	if string(got) != string(original) {
		t.Fatal("bad configuration was overwritten")
	}
}
