package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"openim38.local/identity"
)

func TestGinEntryZoneUsesTrustedRequestForPolicyAndRPC(t *testing.T) {
	for _, row := range []struct{ host, zone, want string }{
		{"192.168.10.11:9885", "lan", "lan"},
		{"im.szputy.com:9885", "lan", "internet"},
		{"192.168.10.11:9885", "", "internet"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "https://"+row.host+"/auth/parse_token", nil)
		c.Request.RemoteAddr = "127.0.0.1:1234"
		c.Request.Header.Set("X-Real-IP", "192.168.14.8")
		c.Request.Header.Set("X-Puty-Entry", "lan")
		if row.zone != "" {
			c.Request.Header.Set("X-Puty-Access-Zone", row.zone)
		}
		// These client-controlled headers are never promoted into RPC facts.
		c.Request.Header.Set("customHeader", identity.EntryZoneMetadataKey)
		c.Request.Header.Set(identity.EntryZoneMetadataKey, "lan")
		bindRequestEntry(c)
		bindRequestEntry(c)
		if identity.EntryZoneFromContext(c) != row.want || identity.EntryZoneFromContext(c.Request.Context()) != row.want {
			t.Fatal("Gin policy and RPC contexts disagree", row)
		}
		keys := c.Value("customHeader").([]string)
		if len(keys) != 1 || keys[0] != identity.EntryZoneMetadataKey {
			t.Fatal("RPC entry metadata dropped or duplicated")
		}
	}
}
