package system_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/suxinwl/GoSuxin/api/admin/system"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/framework/util/gvalid"
	"github.com/suxinwl/GoSuxin/internal/extend/middleware"
	"github.com/suxinwl/GoSuxin/utility/gf"
)

func TestResetPasswordActualValidatorExactLengthAndRedaction(t *testing.T) {
	for _, length := range []int{0, 31, 32, 33} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			password := strings.Repeat("a", length)
			err := gvalid.New().Data(system.AccountResetPasswordReq{ID: 2, Password: password}).Run(context.Background())
			if length == 32 {
				if err != nil {
					t.Fatal("valid 32-character transport digest was rejected")
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "重置密码参数无效") {
				t.Fatal("invalid length must use the fixed Chinese validation message")
			}
			if length > 0 && strings.Contains(err.Error(), password) {
				t.Fatal("validation error exposed the supplied digest")
			}
		})
	}
}

// Bind the real request DTO through the normal HTTP controller binder and the
// production response middleware. The leaf only counts accepted requests: this
// boundary test deliberately issues no database reset or credential changes.
type resetValidationProbe struct{ calls atomic.Int32 }

func (p *resetValidationProbe) AccountResetPassword(_ context.Context, _ *system.AccountResetPasswordReq) (*system.SecurityRes, error) {
	p.calls.Add(1)
	return &system.SecurityRes{R: gf.Success().SetData(map[string]bool{"accepted": true})}, nil
}

func TestResetPasswordNormalHTTPRouteValidatesBeforeHandlerWithoutEcho(t *testing.T) {
	probe := &resetValidationProbe{}
	s := g.Server(guid.S())
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.SetLogStdout(false)
	s.SetFileServerEnabled(false)
	s.Group("/admin", func(group *ghttp.RouterGroup) {
		group.Middleware(middleware.HandlerResponse)
		group.Bind(probe)
	})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	for _, length := range []int{0, 31, 32, 33} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			password := strings.Repeat("b", length)
			payload, err := json.Marshal(map[string]any{"id": 2, "password": password})
			if err != nil {
				t.Fatal(err)
			}
			before := probe.calls.Load()
			res, err := http.Post(fmt.Sprintf("http://127.0.0.1:%d/admin/system/account/resetPassword", s.GetListenedPort()), "application/json", bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			_ = res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			var reply struct {
				Code    int             `json:"code"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data"`
			}
			if err = json.Unmarshal(body, &reply); err != nil {
				t.Fatal("normal route did not return JSON")
			}
			if length > 0 && bytes.Contains(body, []byte(password)) {
				t.Fatal("HTTP response exposed the supplied digest")
			}
			if length == 32 {
				if res.StatusCode != 200 || reply.Code != 0 || probe.calls.Load() != before+1 {
					t.Fatal("valid digest did not reach the normal handler")
				}
			} else if res.StatusCode != 200 || reply.Code != 1 || probe.calls.Load() != before ||
				reply.Message != "重置密码参数无效" || string(reply.Data) != "false" {
				t.Fatal("invalid digest must preserve the production code/message/boolean-false response without invoking the handler")
			}
		})
	}
}
