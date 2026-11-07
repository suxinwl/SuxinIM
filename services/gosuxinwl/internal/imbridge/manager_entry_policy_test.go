package imbridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
)

// BindManager is compiled into the independent biz-suite worker, not the host.
// Exercise its real wrapper and network handler over HTTP. The host identity
// boundary is a fixture here; no real admin/employee session is created.
func TestManagerNetworkRouteUsesCurrentEntryRatherThanSourceCIDR(t *testing.T) {
	t.Setenv("GOSUXIN_HRM_PUBLIC_ADMINS_ONLY", "false")
	_, store, remote := fixture(t)
	svc := &Service{Store: store, Remote: remote, Cache: newFakeCache()}
	server := g.Server(guid.S())
	server.SetAddr("127.0.0.1:0")
	server.SetDumpRouterMap(false)
	server.Group("/", func(group *ghttp.RouterGroup) {
		bindManager(group, func(context.Context) (*Service, func(), error) {
			return svc, func() {}, nil
		})
	})
	must(t, server.Start())
	t.Cleanup(func() { _ = server.Shutdown() })
	for _, tc := range []struct {
		name, host, source, zone string
		duplicate                bool
		want                     string
	}{
		{"HRM IP cross subnet", "192.168.10.11:9880", "192.168.14.8", "lan", false, "lan"},
		{"IM IP cross subnet", "192.168.10.11:9885", "192.168.14.8", "lan", false, "lan"},
		{"HRM domain same source", "hr.szputy.com:9880", "192.168.14.8", "internet", false, "internet"},
		{"HRM domain old CIDR source", "hr.szputy.com:9880", "192.168.10.8", "internet", false, "internet"},
		{"domain forged LAN marker", "im.szputy.com:9885", "192.168.10.8", "lan", false, "internet"},
		{"IP missing marker", "192.168.10.11:9880", "192.168.10.8", "", false, "internet"},
		{"IP duplicate marker", "192.168.10.11:9880", "192.168.10.8", "lan", true, "internet"},
		{"IP public source", "192.168.10.11:9880", "203.0.113.8", "lan", false, "internet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/admin/im/manager/openim/access/networks", server.GetListenedPort()), nil)
			must(t, err)
			r.Host = tc.host
			r.Header.Set("X-GoSuxin-Client-IP", tc.source)
			r.Header.Set("X-Forwarded-For", "192.168.10.99")
			r.Header.Set("X-Real-IP", "192.168.10.99")
			if tc.zone != "" {
				r.Header.Set("X-Puty-Access-Zone", tc.zone)
			}
			if tc.duplicate {
				r.Header.Add("X-Puty-Access-Zone", tc.zone)
			}
			response, err := http.DefaultClient.Do(r)
			must(t, err)
			defer response.Body.Close()
			var envelope struct {
				Data NetworkPolicy `json:"data"`
			}
			must(t, json.NewDecoder(response.Body).Decode(&envelope))
			if response.StatusCode != 200 || envelope.Data.CurrentZone != tc.want || envelope.Data.CurrentIP != tc.source {
				t.Fatalf("manager boundary status=%d, zone=%q want=%q, source preserved=%t", response.StatusCode, envelope.Data.CurrentZone, tc.want, envelope.Data.CurrentIP == tc.source)
			}
		})
	}
}
