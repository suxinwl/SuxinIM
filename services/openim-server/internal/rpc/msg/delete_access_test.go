package msg

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/openimsdk/open-im-server/v3/pkg/common/config"
	"github.com/openimsdk/open-im-server/v3/pkg/common/storage/controller"
	"github.com/openimsdk/open-im-server/v3/pkg/notification"
	"github.com/openimsdk/open-im-server/v3/pkg/rpcli"
	"github.com/openimsdk/protocol/conversation"
	pbmsg "github.com/openimsdk/protocol/msg"
	"github.com/openimsdk/tools/mcontext"
	"github.com/openimsdk/tools/mq/memamq"
	"google.golang.org/grpc"
)

type deleteDatabaseFixture struct {
	controller.CommonMsgDatabase
	physical, logical int
	order             *[]string
}

func (d *deleteDatabaseFixture) DeleteMsgsPhysicalBySeqs(context.Context, string, []int64) error {
	d.physical++
	if d.order != nil {
		*d.order = append(*d.order, "physical-delete")
	}
	return nil
}

func (d *deleteDatabaseFixture) DeleteUserMsgsBySeqs(context.Context, string, string, []int64) error {
	d.logical++
	return nil
}

type deleteConversationFixture struct {
	conversation.ConversationClient
	deny  bool
	order *[]string
}

func (f deleteConversationFixture) GetConversation(_ context.Context, req *conversation.GetConversationReq, _ ...grpc.CallOption) (*conversation.GetConversationResp, error) {
	if f.order != nil {
		*f.order = append(*f.order, "owner-conversation-lookup")
	}
	if f.deny {
		return nil, errors.New("conversation not owned by requester")
	}
	return &conversation.GetConversationResp{Conversation: &conversation.Conversation{OwnerUserID: req.OwnerUserID, ConversationID: req.ConversationID, UserID: "peer", ConversationType: 1}}, nil
}

func deleteTestConfig() *Config {
	return &Config{Share: config.Share{IMAdminUserID: []string{"admin"}}}
}

func TestPhysicalDeleteBySeqRequiresAdministratorBeforeDatabase(t *testing.T) {
	for _, user := range []string{"", "employee", "admin"} {
		db := &deleteDatabaseFixture{}
		s := &msgServer{config: deleteTestConfig(), MsgDatabase: db}
		ctx := mcontext.WithOpUserIDContext(context.Background(), user)
		_, err := s.DeleteMsgPhysicalBySeq(ctx, &pbmsg.DeleteMsgPhysicalBySeqReq{ConversationID: "fixture-conversation", Seqs: []int64{1}})
		if user == "admin" {
			if err != nil || db.physical != 1 {
				t.Fatal("administrator physical deletion was broken", err)
			}
		} else if err == nil || db.physical != 0 {
			t.Fatal("unauthorized caller reached the deletion database")
		}
	}
}

func TestDeleteMsgsRejectsUnownedConversationBeforePhysicalMutation(t *testing.T) {
	db := &deleteDatabaseFixture{}
	order := []string{}
	s := &msgServer{config: deleteTestConfig(), MsgDatabase: db, conversationClient: &rpcli.ConversationClient{ConversationClient: deleteConversationFixture{deny: true, order: &order}}}
	ctx := mcontext.WithOpUserIDContext(context.Background(), "employee")
	_, err := s.DeleteMsgs(ctx, &pbmsg.DeleteMsgsReq{UserID: "employee", ConversationID: "foreign-conversation", Seqs: []int64{1}, DeleteSyncOpt: &pbmsg.DeleteSyncOpt{IsSyncOther: true}})
	if err == nil || db.physical != 0 || db.logical != 0 || !reflect.DeepEqual(order, []string{"owner-conversation-lookup"}) {
		t.Fatal("unowned conversation was modified before its ownership check", err)
	}
	_, err = s.DeleteMsgs(ctx, &pbmsg.DeleteMsgsReq{UserID: "different-owner", ConversationID: "foreign-conversation", Seqs: []int64{1}, DeleteSyncOpt: &pbmsg.DeleteSyncOpt{IsSyncOther: true}})
	if err == nil || db.physical != 0 || len(order) != 1 {
		t.Fatal("forged request owner reached conversation lookup or mutation")
	}
}

func TestDeleteMsgsPreservesOwnedConversationDeletionAndNotification(t *testing.T) {
	order := []string{}
	db := &deleteDatabaseFixture{order: &order}
	queue := memamq.NewMemoryQueue(1, 2)
	defer queue.Stop()
	notified := make(chan *pbmsg.SendMsgReq, 1)
	sender := notification.NewNotificationSender(&config.Notification{}, notification.WithQueue(queue), notification.WithLocalSendMsg(func(_ context.Context, req *pbmsg.SendMsgReq) (*pbmsg.SendMsgResp, error) {
		notified <- req
		return &pbmsg.SendMsgResp{}, nil
	}))
	s := &msgServer{config: deleteTestConfig(), MsgDatabase: db, notificationSender: sender, conversationClient: &rpcli.ConversationClient{ConversationClient: deleteConversationFixture{order: &order}}}
	ctx := mcontext.WithOpUserIDContext(context.Background(), "employee")
	_, err := s.DeleteMsgs(ctx, &pbmsg.DeleteMsgsReq{UserID: "employee", ConversationID: "owned-conversation", Seqs: []int64{2}, DeleteSyncOpt: &pbmsg.DeleteSyncOpt{IsSyncOther: true}})
	if err != nil || db.physical != 1 || !reflect.DeepEqual(order, []string{"owner-conversation-lookup", "physical-delete"}) {
		t.Fatal("owned deletion did not validate before mutation", err)
	}
	select {
	case req := <-notified:
		if req.MsgData.SendID != "employee" || req.MsgData.RecvID != "peer" {
			t.Fatal("valid deletion notification changed participants")
		}
	case <-time.After(time.Second):
		t.Fatal("valid deletion no longer emits its notification")
	}
	_, err = s.DeleteMsgs(ctx, &pbmsg.DeleteMsgsReq{UserID: "employee", ConversationID: "owned-conversation", Seqs: []int64{3}})
	if err != nil || db.logical != 1 || db.physical != 1 {
		t.Fatal("user-only deletion behavior changed", err)
	}
}
