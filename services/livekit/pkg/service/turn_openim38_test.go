package service

import (
	"net"
	"net/netip"
	"testing"

	"github.com/livekit/livekit-server/pkg/config"
	"gopkg.in/yaml.v3"
)

func TestOpenIM38TURNAdvertisedTLSPortPreservesDefaultAndUsesExplicitPort(t *testing.T) {
	for _, tc := range []struct {
		domain string
		port   uint16
		want   string
	}{
		{"192.168.10.11", 0, "turns:192.168.10.11:443?transport=tcp"},
		{"192.168.10.11", 25349, "turns:192.168.10.11:25349?transport=tcp"},
		{"kf.szputy.com", 25349, "turns:kf.szputy.com:25349?transport=tcp"},
		{"2001:db8::1", 25349, "turns:[2001:db8::1]:25349?transport=tcp"},
	} {
		if got := embeddedTURNTLSURL(config.TURNConfig{Domain: tc.domain, AdvertisedTLSPort: tc.port}); got != tc.want {
			t.Fatalf("wrong public TURN URL: %s", got)
		}
	}
	for _, input := range []string{"advertised_tls_port: -1", "advertised_tls_port: 65536"} {
		var c config.TURNConfig
		if err := yaml.Unmarshal([]byte(input), &c); err == nil {
			t.Fatal("invalid advertised port accepted")
		}
	}
}

func TestOpenIM38TURNLocalPeerExceptionIsExactAndRetainsMediaPortPolicy(t *testing.T) {
	local := map[netip.Addr]struct{}{netip.MustParseAddr("172.20.0.3"): {}, netip.MustParseAddr("192.168.10.11"): {}}
	for _, tc := range []struct {
		enabled, standalone bool
		peer                string
		want                bool
	}{
		{false, false, "172.20.0.3", false}, {true, true, "172.20.0.3", false},
		{true, false, "172.20.0.3", true}, {true, false, "::ffff:172.20.0.3", true},
		{true, false, "172.20.0.4", false}, {true, false, "192.168.10.10", false},
		{true, false, "127.0.0.1", false}, {true, false, "bad", false},
	} {
		if got := allowTURNLocalMediaPeer(tc.enabled, tc.standalone, local, net.ParseIP(tc.peer)); got != tc.want {
			t.Fatalf("unexpected exception for %s", tc.peer)
		}
	}
	policy := newTURNRelayPolicy(nil, local, []portRange{{27882, 27882}})
	for _, port := range []int{22, 7880, 27017, 30200, 23478, 27900} {
		if policy.allows(&net.UDPAddr{IP: net.ParseIP("172.20.0.3"), Port: port}) {
			t.Fatalf("local non-media port %d exposed", port)
		}
	}
	if !policy.allows(&net.UDPAddr{IP: net.ParseIP("172.20.0.3"), Port: 27882}) {
		t.Fatal("own RTC media port rejected")
	}
}
