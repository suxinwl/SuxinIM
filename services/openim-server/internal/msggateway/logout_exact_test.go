package msggateway

import (
	"context"
	"testing"

	"github.com/openimsdk/protocol/msggateway"
	"google.golang.org/grpc/metadata"
)

type logoutConnections struct {
	LongConnServer
	clients []*Client
	kicked  []*Client
}

func (s *logoutConnections) GetUserPlatformCons(string, int) ([]*Client, bool, bool) {
	return s.clients, true, true
}
func (s *logoutConnections) KickUserConn(c *Client) error {
	s.kicked = append(s.kicked, c)
	return nil
}

func TestExactLogoutNeverKicksNewSamePlatformConnection(t *testing.T) {
	conns := &logoutConnections{}
	old := &Client{UserID: "alice", PlatformID: 5, token: "old-session", longConnServer: conns}
	newer := &Client{UserID: "alice", PlatformID: 5, token: "new-session", longConnServer: conns}
	conns.clients = []*Client{old, newer}
	s := &Server{LongConnServer: conns}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("openim38-revoke-token", old.token))
	if _, err := s.KickUserOffline(ctx, &msggateway.KickUserOfflineReq{PlatformID: 5, KickUserIDList: []string{"alice"}}); err != nil {
		t.Fatal(err)
	}
	if len(conns.kicked) != 1 || conns.kicked[0] != old {
		t.Fatal("delayed old-token logout kicked the new same-platform connection")
	}
	// Repeating an old cleanup cannot close a newly replaced connection.
	conns.clients, conns.kicked = []*Client{newer}, nil
	_, _ = s.KickUserOffline(ctx, &msggateway.KickUserOfflineReq{PlatformID: 5, KickUserIDList: []string{"alice"}})
	if len(conns.kicked) != 0 {
		t.Fatal("idempotent cleanup matched only by user/platform")
	}
	// Original admin force-logout remains platform-wide when no exact token is set.
	_, _ = s.KickUserOffline(context.Background(), &msggateway.KickUserOfflineReq{PlatformID: 5, KickUserIDList: []string{"alice"}})
	if len(conns.kicked) != 1 || conns.kicked[0] != newer {
		t.Fatal("existing platform-wide admin logout regressed")
	}
}
