package chat

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/openimsdk/chat/pkg/common/constant"
	"github.com/openimsdk/chat/pkg/common/imapi"
	chatpb "github.com/openimsdk/chat/pkg/protocol/chat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"net/http/httptest"
	"strings"
	"testing"
)

type passwordChatClient struct {
	chatpb.ChatClient
	changed    string
	omitHeader bool
}

func (c *passwordChatClient) ResetPassword(_ context.Context, _ *chatpb.ResetPasswordReq, opts ...grpc.CallOption) (*chatpb.ResetPasswordResp, error) {
	if !c.omitHeader {
		for _, opt := range opts {
			if header, ok := opt.(grpc.HeaderCallOption); ok {
				*header.HeaderAddr = metadata.Pairs("openim38-reset-user-id", "verified-user")
			}
		}
	}
	return &chatpb.ResetPasswordResp{}, nil
}
func (c *passwordChatClient) ChangePassword(_ context.Context, req *chatpb.ChangePasswordReq, _ ...grpc.CallOption) (*chatpb.ChangePasswordResp, error) {
	c.changed = req.UserID
	return &chatpb.ChangePasswordResp{}, nil
}

type passwordIM struct {
	imapi.CallerInterface
	user string
	fail bool
}

func (c *passwordIM) ImAdminTokenWithDefaultAdmin(context.Context) (string, error) {
	return "internal-token", nil
}
func (c *passwordIM) ForceOffLine(_ context.Context, user string) error {
	c.user = user
	if c.fail {
		return errors.New("IM unavailable")
	}
	return nil
}
func TestResetUsesVerifiedRPCIdentityAndReportsRevocationFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name       string
		omit, fail bool
		wantCode   int
	}{{"success", false, false, 0}, {"identity missing", true, false, 500}, {"revocation failed", false, true, 500}} {
		t.Run(tc.name, func(t *testing.T) {
			client := &passwordChatClient{omitHeader: tc.omit}
			im := &passwordIM{fail: tc.fail}
			api := &Api{chatClient: client, imApiCaller: im}
			r := gin.New()
			r.POST("/reset", api.ResetPassword)
			req := httptest.NewRequest("POST", "/reset", strings.NewReader(`{"email":"fixture@openim38.test","verifyCode":"test-code","password":"digest","userID":"attacker-controlled"}`))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			var result struct {
				ErrCode int `json:"errCode"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if tc.wantCode == 0 && result.ErrCode != 0 || tc.wantCode != 0 && result.ErrCode == 0 {
				t.Fatalf("unexpected error code %d", result.ErrCode)
			}
			if !tc.omit && im.user != "verified-user" {
				t.Fatalf("unverified revocation identity: %q", im.user)
			}
			if tc.omit && im.user != "" {
				t.Fatal("revocation attempted with missing verified identity")
			}
		})
	}
}
func TestChangePasswordWithoutUserIDRevokesAuthenticatedUser(t *testing.T) {
	client := &passwordChatClient{}
	im := &passwordIM{}
	api := &Api{chatClient: client, imApiCaller: im}
	r := gin.New()
	r.POST("/change", func(c *gin.Context) {
		c.Set(constant.RpcOpUserID, "authenticated-user")
		c.Set(constant.RpcOpUserType, []string{"1"})
		api.ChangePassword(c)
	})
	req := httptest.NewRequest("POST", "/change", strings.NewReader(`{"currentPassword":"old","newPassword":"new"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if client.changed != "authenticated-user" || im.user != client.changed {
		t.Fatalf("changed=%q revoked=%q", client.changed, im.user)
	}
}
