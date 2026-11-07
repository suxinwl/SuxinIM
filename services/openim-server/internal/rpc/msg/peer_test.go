package msg

import (
	"context"
	"errors"
	"testing"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/controller"
	"github.com/openimsdk/open-im-server/v3/pkg/rpccache"
	"github.com/openimsdk/open-im-server/v3/pkg/rpcli"
	"github.com/openimsdk/protocol/constant"
	pbmsg "github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/protocol/relation"
	"github.com/openimsdk/protocol/sdkws"
	"github.com/openimsdk/protocol/user"
	"github.com/openimsdk/tools/mcontext"
	"google.golang.org/grpc"
)

type colleagueRelations struct {
	relation.FriendClient
	black                     bool
	friendChecks, blackChecks int
}

func (f *colleagueRelations) IsFriend(context.Context, *relation.IsFriendReq, ...grpc.CallOption) (*relation.IsFriendResp, error) {
	f.friendChecks++
	return &relation.IsFriendResp{}, nil
}
func (f *colleagueRelations) IsBlack(context.Context, *relation.IsBlackReq, ...grpc.CallOption) (*relation.IsBlackResp, error) {
	f.blackChecks++
	return &relation.IsBlackResp{InUser2Blacks: f.black}, nil
}

type colleagueUsers struct{ user.UserClient }

func (f colleagueUsers) GetGlobalRecvMessageOpt(context.Context, *user.GetGlobalRecvMessageOptReq, ...grpc.CallOption) (*user.GetGlobalRecvMessageOptResp, error) {
	return &user.GetGlobalRecvMessageOptResp{GlobalRecvMsgOpt: constant.NotReceiveMessage}, nil
}

type colleagueDatabase struct {
	controller.CommonMsgDatabase
	writes int
}

func (d *colleagueDatabase) MsgToMQ(context.Context, string, *sdkws.MsgData) error {
	d.writes++
	return nil
}

func colleagueServer(t *testing.T, black bool) (*msgServer, *colleagueRelations, *colleagueDatabase, context.Context, *pbmsg.SendMsgReq) {
	t.Helper()
	t.Setenv("HRM_SSO_REQUIRED", "true")
	friends := &colleagueRelations{black: black}
	db := &colleagueDatabase{}
	s := &msgServer{config: &Config{Share: config.Share{IMAdminUserID: []string{"admin"}}, RpcConfig: config.Msg{FriendVerify: true}},
		FriendLocalCache: rpccache.NewFriendLocalCache(&rpcli.RelationClient{FriendClient: friends}, &config.LocalCache{}, nil),
		UserLocalCache:   rpccache.NewUserLocalCache(&rpcli.UserClient{UserClient: colleagueUsers{}}, &config.LocalCache{}, nil),
		MsgDatabase:      db, peerAuthorize: func(context.Context, string, string) error { return nil }}
	ctx := mcontext.WithOpUserIDContext(context.Background(), "emp_sender")
	req := &pbmsg.SendMsgReq{MsgData: &sdkws.MsgData{SendID: "emp_sender", RecvID: "emp_peer", SessionType: constant.SingleChatType, ContentType: constant.Text}}
	return s, friends, db, ctx, req
}

func TestEligibleColleaguesSkipOnlyFriendshipAndRetainBlacklist(t *testing.T) {
	s, friends, _, ctx, req := colleagueServer(t, false)
	if err := s.messageVerification(ctx, req); err != nil || friends.friendChecks != 0 || friends.blackChecks != 1 {
		t.Fatal("colleague exception bypassed blacklist or still requires friendship", err)
	}
	s, friends, db, ctx, req := colleagueServer(t, true)
	if _, err := s.sendMsgSingleChat(ctx, req); err == nil || friends.blackChecks != 1 || friends.friendChecks != 0 || db.writes != 0 {
		t.Fatal("blocked colleague reached message persistence", err)
	}
}

func TestColleagueDenialAndForgedSenderNeverReachPersistenceOrNotificationBypass(t *testing.T) {
	s, friends, db, ctx, req := colleagueServer(t, false)
	s.peerAuthorize = func(context.Context, string, string) error { return errors.New("HRM denies target") }
	for _, content := range []int32{constant.Text, constant.NotificationBegin} {
		req.MsgData.ContentType = content
		if _, err := s.sendMsgSingleChat(ctx, req); err == nil || friends.blackChecks != 0 || db.writes != 0 {
			t.Fatal("denied colleague bypassed live eligibility")
		}
	}
	req.MsgData.SendID = "admin"
	if _, err := s.sendMsgSingleChat(ctx, req); err == nil || db.writes != 0 {
		t.Fatal("forged administrator identity bypassed employee binding")
	}
}

func TestColleagueChatRetainsRecipientRefusal(t *testing.T) {
	s, _, db, ctx, req := colleagueServer(t, false)
	if response, err := s.sendMsgSingleChat(ctx, req); err != nil || response != nil || db.writes != 0 {
		t.Fatal("colleague exception bypassed recipient receive preference", err)
	}
}

func TestLegacyFriendVerificationAndTrustedAdminNotificationRemain(t *testing.T) {
	s, friends, _, ctx, req := colleagueServer(t, false)
	t.Setenv("HRM_SSO_REQUIRED", "false")
	if err := s.messageVerification(ctx, req); err == nil || friends.friendChecks != 1 {
		t.Fatal("legacy friendship policy changed")
	}
	t.Setenv("HRM_SSO_REQUIRED", "true")
	ctx = mcontext.WithOpUserIDContext(ctx, "admin")
	req.MsgData.ContentType = constant.NotificationBegin
	if err := s.messageVerification(ctx, req); err != nil {
		t.Fatal("trusted administrator notification was broken", err)
	}
}

func TestServerGeneratedReadAndFriendNotificationsDoNotRequireClientSession(t *testing.T) {
	s, _, _, ctx, req := colleagueServer(t, false)
	s.peerAuthorize = func(context.Context, string, string) error {
		t.Error("server notification used employee direct-chat authorization")
		return errors.New("no client SID on server event")
	}
	for _, content := range []int32{constant.HasReadReceipt, constant.FriendAddedNotification} {
		req.MsgData.ContentType, req.MsgData.MsgFrom = content, constant.SysMsgType
		if err := s.messageVerification(ctx, req); err != nil {
			t.Fatal("server notification was broken", err)
		}
	}
}
