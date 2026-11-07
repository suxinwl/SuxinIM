package im

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var (
	ErrForbidden = errors.New("not a participant or operation is not permitted")
	ErrInvalid   = errors.New("invalid input")
	ErrConflict  = errors.New("client_message_id already used for different message")
)

type MessageInput struct {
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	ReceiverID       int64  `json:"receiver_id"`
	ClientMessageID  string `json:"client_message_id"`
	MessageType      string `json:"message_type"`
	Content          string `json:"content"`
}

func model(ctx context.Context, table string) *gdb.Model { return core.Model(table).Ctx(ctx) }

func streamID(kind, target string, user int64) (string, error) {
	if user <= 0 || target == "" || len(target) > 64 {
		return "", ErrInvalid
	}
	switch kind {
	case "private":
		peer, err := strconv.ParseInt(target, 10, 64)
		if err != nil || peer <= 0 || peer == user {
			return "", ErrInvalid
		}
		if peer < user {
			return fmt.Sprintf("private:%d:%d", peer, user), nil
		}
		return fmt.Sprintf("private:%d:%d", user, peer), nil
	case "group", "channel":
		return kind + ":" + target, nil
	default:
		return "", ErrInvalid
	}
}

func participants(ctx context.Context, user int64, kind, target string) ([]int64, error) {
	if _, err := streamID(kind, target, user); err != nil {
		return nil, err
	}
	if kind == "private" {
		peer, _ := strconv.ParseInt(target, 10, 64)
		if isHistoricalImport(ctx) {
			return historicalPrivateParticipants(ctx, user, target)
		}
		if err := privateCommunication(ctx, user, peer, false); err != nil {
			return nil, err
		}
		return []int64{user, peer}, nil
	}
	if !isHistoricalImport(ctx) {
		if err := communicationAccount(ctx, user, false); err != nil {
			return nil, err
		}
	} else {
		exists, err := core.HostModel("admin").Ctx(ctx).Where("id", user).WhereNull("deletetime").Exist()
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, ErrForbidden
		}
	}
	n, err := model(ctx, "biz_im_"+kind).Where(g.Map{"id": target, "status": "active"}).Count()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrForbidden
	}
	rows, err := model(ctx, "biz_im_"+kind+"_member").Where(g.Map{kind + "_id": target, "status": "active"}).All()
	if err != nil {
		return nil, err
	}
	users := make([]int64, 0, len(rows))
	found := false
	for _, row := range rows {
		id := row["user_id"].Int64()
		users = append(users, id)
		found = found || id == user
	}
	if !found {
		return nil, ErrForbidden
	}
	return users, nil
}

func SendMessage(ctx context.Context, user int64, in MessageInput) (g.Map, error) {
	return sendMessage(ctx, user, in, "", true)
}

func sendMessage(ctx context.Context, user int64, in MessageInput, sourceTime string, push bool) (g.Map, error) {
	if !isHistoricalImport(ctx) {
		if err := currentAccount(ctx, user, true); err != nil {
			return nil, err
		}
	}
	if in.ConversationType == "private" && in.ConversationID == "" {
		in.ConversationID = strconv.FormatInt(in.ReceiverID, 10)
	}
	if !isHistoricalImport(ctx) && in.ConversationType != "private" {
		if err := communicationAccount(ctx, user, true); err != nil {
			return nil, err
		}
	}
	stream, err := streamID(in.ConversationType, in.ConversationID, user)
	if err != nil {
		return nil, err
	}
	if in.ClientMessageID == "" || len(in.ClientMessageID) > 128 || len(in.Content) == 0 || len(in.Content) > 65536 {
		return nil, ErrInvalid
	}
	if in.MessageType == "" {
		in.MessageType = "text"
	}
	switch in.MessageType {
	case "text", "image", "file", "emoji":
	default:
		return nil, ErrInvalid
	}
	requestBytes, _ := json.Marshal([]string{stream, in.MessageType, in.Content})
	requestDigest := sha256.Sum256(requestBytes)
	requestHash := hex.EncodeToString(requestDigest[:])
	users, err := participants(ctx, user, in.ConversationType, in.ConversationID)
	if err != nil {
		return nil, err
	}
	if in.ConversationType == "group" {
		space, e := model(ctx, "biz_im_group").Where("id", in.ConversationID).One()
		if e != nil {
			return nil, e
		}
		member, e := model(ctx, "biz_im_group_member").Where(g.Map{"group_id": in.ConversationID, "user_id": user}).One()
		if e != nil {
			return nil, e
		}
		if member["role"].String() == "member" && (space["mute_all"].Int() != 0 || member["muted"].Int() != 0) {
			return nil, ErrForbidden
		}
	}
	if in.MessageType == "text" {
		words, e := model(ctx, "biz_im_sensitive_word").Where("status", "active").All()
		if e != nil {
			return nil, e
		}
		for _, w := range words {
			word := w["word"].String()
			if word == "" {
				continue
			}
			replacement := w["replacement"].String()
			if replacement == "" {
				replacement = "***"
			}
			in.Content = strings.ReplaceAll(in.Content, word, replacement)
		}
		if len(in.Content) > 65536 {
			return nil, ErrInvalid
		}
	}
	var out g.Map
	fresh := false
	err = core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		// Increment first to serialize writers and read receipts in every supported DB.
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		// Membership may have changed while waiting for the write transaction.
		if !isHistoricalImport(ctx) {
			if e := currentAccount(ctx, user, true); e != nil {
				return e
			}
		}
		users, err = participants(ctx, user, in.ConversationType, in.ConversationID)
		if err != nil {
			return err
		}
		if !isHistoricalImport(ctx) && in.ConversationType != "private" {
			users = liveRecipients(ctx, users)
		}
		if in.ConversationType == "channel" {
			if e := manager(ctx, user, "channel", in.ConversationID); e != nil {
				return e
			}
		}
		if in.ConversationType == "group" {
			space, e := tx.Model("biz_im_group").Where("id", in.ConversationID).One()
			if e != nil {
				return e
			}
			member, e := tx.Model("biz_im_group_member").Where(g.Map{"group_id": in.ConversationID, "user_id": user}).One()
			if e != nil {
				return e
			}
			if member["role"].String() == "member" && (space["mute_all"].Int() != 0 || member["muted"].Int() != 0) {
				return ErrForbidden
			}
		}
		old, e := tx.Model("biz_im_message").Where(g.Map{"sender_id": user, "client_message_id": in.ClientMessageID}).One()
		if e != nil {
			return e
		}
		if len(old) > 0 {
			if old["request_hash"].String() != requestHash {
				return ErrConflict
			}
			out = old.Map()
			return nil
		}
		seq, e := tx.Model("biz_im_counter").Where("id", 1).Value("value")
		if e != nil {
			return e
		}
		stamp := core.Now()
		if sourceTime != "" {
			stamp = sourceTime
		}
		out = g.Map{"id": guid.S(), "seq": seq.Int64(), "change_seq": seq.Int64(), "sender_id": user, "conversation_type": in.ConversationType, "target_id": in.ConversationID, "stream_id": stream, "client_message_id": in.ClientMessageID, "request_hash": requestHash, "message_type": in.MessageType, "content": in.Content, "send_time": stamp, "status": "sent"}
		if _, e = tx.Model("biz_im_message").Data(out).Insert(); e != nil {
			return e
		}
		for _, recipient := range users {
			target := in.ConversationID
			if in.ConversationType == "private" && recipient != user {
				target = strconv.FormatInt(user, 10)
			}
			q := tx.Model("biz_im_conversation").Where(g.Map{"user_id": recipient, "stream_id": stream})
			previous, e := q.One()
			if e != nil {
				return e
			}
			unread := int64(0)
			if len(previous) > 0 {
				unread = previous["unread_count"].Int64()
			}
			if recipient != user {
				unread++
			}
			data := g.Map{"last_seq": seq.Int64(), "change_seq": seq.Int64(), "last_message_id": out["id"], "last_content": in.Content, "unread_count": unread, "updated_at": stamp}
			if len(previous) == 0 {
				data["id"] = guid.S()
				data["user_id"] = recipient
				data["conversation_type"] = in.ConversationType
				data["conversation_id"] = target
				data["stream_id"] = stream
				_, e = tx.Model("biz_im_conversation").Data(data).Insert()
			} else {
				_, e = q.Update(data)
			}
			if e != nil {
				return e
			}
		}
		fresh = true
		return nil
	})
	if err == nil && fresh && push {
		// The message has committed. A display-name lookup failure must not
		// report the durable send as failed or suppress its realtime delivery.
		_ = decorateRows(ctx, []g.Map{out}, "message")
		delete(out, "request_hash")
		liveHub.publish(liveRecipients(ctx, users), g.Map{"type": "message", "data": out})
	}
	if err == nil && push && !fresh {
		_ = decorateRows(ctx, []g.Map{out}, "message")
		delete(out, "request_hash")
	}
	return out, err
}

func lockCounter(ctx context.Context, tx gdb.TX) error {
	_, err := tx.Exec("UPDATE biz_im_counter SET value = value + 1 WHERE id = 1")
	if err != nil {
		return err
	}
	count, err := tx.Model("biz_im_counter").Where("id", 1).Count()
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = tx.Model("biz_im_counter").Data(g.Map{"id": 1, "value": 1}).Insert()
	}
	return err
}

func Messages(ctx context.Context, user int64, kind, target string, after int64) (gdb.Result, error) {
	if _, err := historyParticipants(ctx, user, kind, target); err != nil {
		return nil, err
	}
	stream, _ := streamID(kind, target, user)
	return model(ctx, "biz_im_message").Where("stream_id", stream).WhereGT("change_seq", after).OrderAsc("change_seq").Limit(200).All()
}

func MarkRead(ctx context.Context, user int64, kind, target string, seq int64) error {
	users, err := historyParticipants(ctx, user, kind, target)
	if err != nil {
		return err
	}
	stream, _ := streamID(kind, target, user)
	if seq <= 0 {
		return ErrInvalid
	}
	advanced := false
	err = core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		users, err = historyParticipants(ctx, user, kind, target)
		if err != nil {
			return err
		}
		exists, e := tx.Model("biz_im_message").Where(g.Map{"stream_id": stream, "seq": seq}).Count()
		if e != nil {
			return e
		}
		if exists == 0 {
			return ErrInvalid
		}
		q := tx.Model("biz_im_conversation").Where(g.Map{"user_id": user, "stream_id": stream})
		row, e := q.One()
		if e != nil {
			return e
		}
		if len(row) == 0 {
			return ErrInvalid
		}
		if row["last_read_seq"].Int64() >= seq {
			// A duplicate or stale acknowledgement must not produce another
			// receipt, otherwise clients can trigger an acknowledgement loop.
			return nil
		}
		n, e := tx.Model("biz_im_message").Where("stream_id", stream).WhereGT("seq", seq).WhereNot("sender_id", user).Count()
		if e != nil {
			return e
		}
		change, e := tx.Model("biz_im_counter").Where("id", 1).Value("value")
		if e != nil {
			return e
		}
		_, e = q.Update(g.Map{"last_read_seq": seq, "change_seq": change.Int64(), "unread_count": n, "updated_at": core.Now()})
		advanced = e == nil
		return e
	})
	if err == nil && advanced {
		if kind == "private" {
			peer, _ := strconv.ParseInt(target, 10, 64)
			if privateCommunication(ctx, user, peer, false) != nil {
				users = []int64{user}
			}
		}
		liveHub.publish(liveRecipients(ctx, users), g.Map{"type": "receipt", "data": g.Map{"user_id": user, "stream_id": stream, "last_read_seq": seq}})
	}
	return err
}

// ImportMessage imports already mapped historical messages after validating
// historical identities and group/channel membership, independently of current
// communication permissions or friendships. It preserves deduplication and
// conversation invariants without broadcasting a live event.
// ClientMessageID must be "ruoyi:<source-id>".
func ImportMessage(ctx context.Context, user int64, in MessageInput) (g.Map, error) {
	ctx = context.WithValue(ctx, historicalImportKey{}, true)
	return sendMessage(ctx, user, in, "", false)
}

// ImportHistoricalMessage preserves the source timestamp. Import source rows in
// chronological order so sequence-based conversation previews follow history.
// Imported history is not broadcast as a new live event.
func ImportHistoricalMessage(ctx context.Context, user int64, in MessageInput, sendTime string) (g.Map, error) {
	ctx = context.WithValue(ctx, historicalImportKey{}, true)
	stamp, err := core.ParseTime(sendTime)
	if err != nil {
		return nil, ErrInvalid
	}
	if !strings.HasPrefix(in.ClientMessageID, "ruoyi:") {
		return nil, ErrInvalid
	}
	return sendMessage(ctx, user, in, stamp.Format("2006-01-02 15:04:05"), false)
}

// ValidateImport is read-only and shares the import's message, membership and
// idempotency requirements. Apply rechecks authorization inside its transaction.
func ValidateImport(ctx context.Context, user int64, in MessageInput, sendTime string) error {
	ctx = context.WithValue(ctx, historicalImportKey{}, true)
	if _, err := core.ParseTime(sendTime); err != nil {
		return ErrInvalid
	}
	if !strings.HasPrefix(in.ClientMessageID, "ruoyi:") || len(in.ClientMessageID) > 128 || len(in.Content) == 0 || len(in.Content) > 65536 {
		return ErrInvalid
	}
	if in.ConversationType == "private" && in.ConversationID == "" {
		in.ConversationID = strconv.FormatInt(in.ReceiverID, 10)
	}
	if in.MessageType == "" {
		in.MessageType = "text"
	}
	switch in.MessageType {
	case "text", "image", "file", "emoji":
	default:
		return ErrInvalid
	}
	stream, err := streamID(in.ConversationType, in.ConversationID, user)
	if err != nil {
		return err
	}
	if _, err = participants(ctx, user, in.ConversationType, in.ConversationID); err != nil {
		return err
	}
	if in.ConversationType == "channel" {
		if err = manager(ctx, user, "channel", in.ConversationID); err != nil {
			return err
		}
	}
	if in.ConversationType == "group" {
		space, e := model(ctx, "biz_im_group").Where("id", in.ConversationID).One()
		if e != nil {
			return e
		}
		member, e := model(ctx, "biz_im_group_member").Where(g.Map{"group_id": in.ConversationID, "user_id": user}).One()
		if e != nil {
			return e
		}
		if member["role"].String() == "member" && (space["mute_all"].Int() != 0 || member["muted"].Int() != 0) {
			return ErrForbidden
		}
	}
	old, err := model(ctx, "biz_im_message").Where(g.Map{"sender_id": user, "client_message_id": in.ClientMessageID}).One()
	if err != nil {
		return err
	}
	request, _ := json.Marshal([]string{stream, in.MessageType, in.Content})
	digest := sha256.Sum256(request)
	if len(old) > 0 && old["request_hash"].String() != hex.EncodeToString(digest[:]) {
		return ErrConflict
	}
	return nil
}
