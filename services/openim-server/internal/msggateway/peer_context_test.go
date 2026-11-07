package msggateway

import (
	"context"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/openimsdk/open-im-server/v3/pkg/rpcli"
	"github.com/openimsdk/protocol/constant"
	pbmsg "github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/tools/mcontext"
	"github.com/openimsdk/tools/mw"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
	"openim38.local/identity"
)

func TestSocketContextRetainsEntryZoneForPeriodicPolicyChecks(t *testing.T) {
	r := httptest.NewRequest("GET", "https://192.168.10.11:9885/ws", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Real-IP", "192.168.14.8")
	r.Header.Set("X-Puty-Entry", "lan")
	r.Header.Set("X-Puty-Access-Zone", "lan")
	r = r.WithContext(identity.WithRequestEntry(r.Context(), r))
	connection := newContext(httptest.NewRecorder(), r)
	heartbeat, cancel := context.WithCancel(connection)
	defer cancel()
	check, done := context.WithTimeout(heartbeat, time.Second)
	defer done()
	if identity.EntryZoneFromContext(connection) != "lan" || identity.EntryZoneFromContext(check) != "lan" {
		t.Fatal("socket lease/heartbeat silently reverted to public or source-IP policy")
	}
}

type peerMessageServer struct {
	pbmsg.UnimplementedMsgServer
	seen chan context.Context
}

func (s *peerMessageServer) SendMsg(ctx context.Context, req *pbmsg.SendMsgReq) (*pbmsg.SendMsgResp, error) {
	s.seen <- ctx
	return &pbmsg.SendMsgResp{ClientMsgID: req.MsgData.ClientMsgID}, nil
}

func TestColleagueFactsCrossTheLockedRPCInterceptors(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(mw.GrpcServer())
	implementation := &peerMessageServer{seen: make(chan context.Context, 1)}
	pbmsg.RegisterMsgServer(server, implementation)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connection, err := grpc.DialContext(ctx, "bufnet", grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }), grpc.WithTransportCredentials(insecure.NewCredentials()), mw.GrpcClient(), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	handler := NewGrpcHandler(validator.New(), &rpcli.MsgClient{MsgClient: pbmsg.NewMsgClient(connection)}, nil)
	bound := identity.WithClientSession(identity.WithEntryZone(mcontext.WithMustInfoCtx([]string{"peer-test", "emp_sender", constant.PlatformIDToName(3), "fixture-connection"}), "lan"), identity.Session{SessionID: "session-actual-sid-000001", Platform: 3}, "192.168.14.8")
	// The payload's platform is deliberately different: authorization reads the
	// SID's server-owned platform, never the user-controlled MsgData field.
	body, _ := proto.Marshal(&sdkws.MsgData{SendID: "emp_sender", RecvID: "emp_peer", ClientMsgID: "fixture-client-message", SessionType: constant.SingleChatType, ContentType: constant.Text, SenderPlatformID: 2, Content: []byte(`{"content":"fixture"}`)})
	if _, err = handler.SendMessage(bound, &Req{Data: body}); err != nil {
		t.Fatal(err)
	}
	select {
	case received := <-implementation.seen:
		for key, expected := range map[string]string{"openim38-hrm-client-session": "session-actual-sid-000001", "openim38-hrm-client-address": "192.168.14.8", identity.EntryZoneMetadataKey: "lan"} {
			values, _ := received.Value(key).([]string)
			if len(values) != 1 || values[0] != expected {
				t.Fatal("authenticated gateway facts dropped by RPC middleware", key)
			}
		}
		if mcontext.GetOpUserID(received) != "emp_sender" {
			t.Fatal("RPC actor changed")
		}
	case <-ctx.Done():
		t.Fatal("RPC did not receive bound client facts")
	}
}

func TestGatewayRejectsForgedInnerSenderBeforeRPC(t *testing.T) {
	t.Setenv("HRM_SSO_REQUIRED", "true")
	handler := NewGrpcHandler(validator.New(), nil, nil)
	ctx := mcontext.WithOpUserIDContext(context.Background(), "emp_sender")
	for _, forged := range []string{"admin", "emp_other"} {
		body, _ := proto.Marshal(&sdkws.MsgData{SendID: forged, RecvID: "emp_peer", SessionType: constant.SingleChatType, ContentType: constant.Text})
		if _, err := handler.SendMessage(ctx, &Req{Data: body}); err == nil {
			t.Fatal("forged message sender reached RPC")
		}
	}
	body, _ := proto.Marshal(&sdkws.MsgData{SendID: "emp_sender", RecvID: "emp_peer", SessionType: constant.SingleChatType, ContentType: constant.FriendAddedNotification, MsgFrom: constant.SysMsgType})
	if _, err := handler.SendMessage(ctx, &Req{Data: body}); err == nil {
		t.Fatal("client forged a server-owned notification")
	}
}
