package mw

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/protocol/admin"
	"google.golang.org/grpc"
	"openim38.local/identity"
)

type entryParseClient struct{ admin.AdminClient }

func (entryParseClient) ParseToken(context.Context, *admin.ParseTokenReq, ...grpc.CallOption) (*admin.ParseTokenResp, error) {
	return &admin.ParseTokenResp{UserID: "synthetic-entry-user", UserType: constant.NormalUser}, nil
}

func TestChatTokenMiddlewareCarriesTrustedEntryToRPCWithoutClientZone(t *testing.T) {
	// This isolates the real HTTP middleware/RPC-context wiring. Identity and
	// signed online policy checks are covered independently with the live fixture.
	t.Setenv("HRM_SSO_REQUIRED", "false")
	for _, row := range []struct{ host, entry, zone, want string }{
		{"192.168.10.11:9885", "lan", "lan", "lan"},
		{"im.szputy.com:9885", "internet", "internet", "internet"},
		{"im.szputy.com:9885", "lan", "lan", "internet"},
		{"192.168.10.11:9885", "lan", "", "internet"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "https://"+row.host+"/chat/user/info", nil)
		c.Request.RemoteAddr = "127.0.0.1:1234"
		c.Request.Header.Set("X-Real-IP", "192.168.14.8")
		c.Request.Header.Set("X-Puty-Entry", row.entry)
		c.Request.Header.Set("X-Puty-Access-Zone", row.zone)
		c.Request.Header.Set("token", "synthetic-token")
		c.Request.Header.Set("customHeader", identity.EntryZoneMetadataKey)
		c.Request.Header.Set(identity.EntryZoneMetadataKey, "lan")
		New(entryParseClient{}).CheckToken(c)
		if c.IsAborted() || identity.EntryZoneFromContext(c.Request.Context()) != row.want || identity.EntryZoneFromContext(c) != row.want {
			t.Fatal("Chat HTTP policy and outgoing RPC zone disagree", row)
		}
		keys := c.Value(constant.RpcCustomHeader).([]string)
		if len(keys) != 2 || keys[0] != constant.RpcOpUserType || keys[1] != identity.EntryZoneMetadataKey {
			t.Fatal("Chat actor metadata overwritten or zone dropped")
		}
	}
}
