package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHRMObjectPrefixUsesOnlyTrustedEntry(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	t.Setenv("TRUSTED_ENTRY_PROXY_CIDRS", "172.30.38.1/32")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "http://untrusted.example/object/complete_multipart_upload", nil)
	c.Request.RemoteAddr = "172.30.38.1:1234"
	c.Request.Header.Set("X-Request-Api", "https://attacker.example/api")
	c.Request.Header.Set("X-Puty-Entry", "lan")
	var prefix string
	if err := setURLPrefix(c, &prefix); err != nil || prefix != "https://192.168.10.11:9885/api/object/" {
		t.Fatal("attachment URL trusted client-provided origin", prefix, err)
	}
	c.Request.RemoteAddr = "192.0.2.5:1234"
	if err := setURLPrefix(c, &prefix); err == nil {
		t.Fatal("untrusted peer selected an entry profile")
	}
}
