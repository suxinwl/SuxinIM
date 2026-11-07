package im

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"strconv"
	"time"
)

type GroupAction struct {
	Action    string `json:"action"`
	UserID    int64  `json:"user_id"`
	Enabled   bool   `json:"enabled"`
	Name      string `json:"name"`
	Notice    string `json:"notice"`
	MessageID string `json:"message_id"`
}

// ChangeGroup implements owner/admin/member transitions in one transaction.
func ChangeGroup(ctx context.Context, user int64, id string, in GroupAction) error {
	if err := currentAccount(ctx, user, true); err != nil {
		return err
	}
	var recipients []int64
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		var err error
		recipients, err = participants(ctx, user, "group", id)
		if err != nil {
			return err
		}
		member, err := tx.Model("biz_im_group_member").Where(g.Map{"group_id": id, "user_id": user, "status": "active"}).One()
		if err != nil {
			return err
		}
		role := member["role"].String()
		owner := role == "owner"
		admin := owner || role == "admin"
		target, err := tx.Model("biz_im_group_member").Where(g.Map{"group_id": id, "user_id": in.UserID, "status": "active"}).One()
		if err != nil {
			return err
		}
		group := tx.Model("biz_im_group").Where("id", id)
		switch in.Action {
		case "update":
			if !admin {
				return ErrForbidden
			}
			if in.Name == "" || len(in.Name) > 160 || len(in.Notice) > 500 {
				return ErrInvalid
			}
			_, err = group.Update(g.Map{"name": in.Name, "notice": in.Notice, "updated_at": core.Now()})
		case "mute-all":
			if !admin {
				return ErrForbidden
			}
			_, err = group.Update(g.Map{"mute_all": in.Enabled, "updated_at": core.Now()})
		case "mute-member", "kick":
			if !admin || len(target) == 0 || in.UserID == user || target["role"].String() == "owner" || (!owner && target["role"].String() == "admin") {
				return ErrForbidden
			}
			data := g.Map{"muted": in.Enabled}
			if in.Action == "kick" {
				data = g.Map{"status": "removed"}
			}
			_, err = tx.Model("biz_im_group_member").Where("id", target["id"].String()).Update(data)
			if err == nil && in.Action == "kick" {
				_, err = tx.Model("biz_im_conversation").Where(g.Map{"user_id": in.UserID, "stream_id": "group:" + id}).Delete()
			}
		case "set-admin":
			if !owner || len(target) == 0 || in.UserID == user {
				return ErrForbidden
			}
			newRole := "member"
			if in.Enabled {
				newRole = "admin"
			}
			_, err = tx.Model("biz_im_group_member").Where("id", target["id"].String()).Update(g.Map{"role": newRole})
		case "transfer-owner":
			if !owner || len(target) == 0 || in.UserID == user {
				return ErrForbidden
			}
			if _, err = tx.Model("biz_im_group_member").Where("id", target["id"].String()).Update(g.Map{"role": "owner"}); err != nil {
				return err
			}
			if _, err = tx.Model("biz_im_group_member").Where("id", member["id"].String()).Update(g.Map{"role": "member"}); err != nil {
				return err
			}
			_, err = group.Update(g.Map{"owner_id": in.UserID, "updated_at": core.Now()})
		case "quit":
			if owner {
				return ErrForbidden
			}
			_, err = tx.Model("biz_im_group_member").Where("id", member["id"].String()).Update(g.Map{"status": "left"})
			if err == nil {
				_, err = tx.Model("biz_im_conversation").Where(g.Map{"user_id": user, "stream_id": "group:" + id}).Delete()
			}
		case "dissolve":
			if !owner {
				return ErrForbidden
			}
			_, err = group.Update(g.Map{"status": "dissolved", "updated_at": core.Now()})
			if err == nil {
				_, err = tx.Model("biz_im_conversation").Where("stream_id", "group:"+id).Delete()
			}
		case "pin":
			if !admin {
				return ErrForbidden
			}
			if in.MessageID != "" {
				n, e := tx.Model("biz_im_message").Where(g.Map{"id": in.MessageID, "stream_id": "group:" + id, "status": "sent"}).Count()
				if e != nil {
					return e
				}
				if n == 0 {
					return ErrInvalid
				}
			}
			_, err = group.Update(g.Map{"pinned_message_id": in.MessageID, "updated_at": core.Now()})
		case "invite":
			if !admin || in.UserID <= 0 {
				return ErrForbidden
			}
			if e := communicationAccount(ctx, in.UserID, false); e != nil {
				return e
			}
			q := tx.Model("biz_im_group_member").Where(g.Map{"group_id": id, "user_id": in.UserID})
			previous, e := q.One()
			if e != nil {
				return e
			}
			if len(previous) > 0 {
				if previous["status"].String() != "active" {
					_, err = q.Update(g.Map{"status": "active", "role": "member", "muted": 0, "joined_at": core.Now()})
				}
			} else {
				_, err = tx.Model("biz_im_group_member").Data(g.Map{"id": guid.S(), "group_id": id, "user_id": in.UserID, "role": "member", "status": "active", "joined_at": core.Now()}).Insert()
			}
			recipients = append(recipients, in.UserID)
		default:
			return ErrInvalid
		}
		return err
	})
	if err == nil {
		liveHub.publish(liveRecipients(ctx, recipients), g.Map{"type": "membership", "data": g.Map{"group_id": id, "action": in.Action, "user_id": in.UserID}})
	}
	return err
}

func ChangeFriend(ctx context.Context, user, peer int64, action, displayName string) error {
	return contactData(ctx, user, peer, action, displayName)
}

func RecallMessage(ctx context.Context, user int64, id string) error {
	if err := currentAccount(ctx, user, true); err != nil {
		return err
	}
	var recipients []int64
	var event g.Map
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		row, err := tx.Model("biz_im_message").Where("id", id).One()
		if err != nil {
			return err
		}
		if len(row) == 0 {
			return ErrInvalid
		}
		kind, target := row["conversation_type"].String(), row["target_id"].String()
		if kind == "private" && row["sender_id"].Int64() != user {
			target = strconv.FormatInt(row["sender_id"].Int64(), 10)
		}
		recipients, err = historyParticipants(ctx, user, kind, target)
		if err != nil {
			return err
		}
		if row["sender_id"].Int64() != user {
			return ErrForbidden
		}
		if row["status"].String() == "recalled" {
			return nil
		}
		sent, err := core.ParseTime(row["send_time"].String())
		if err != nil {
			return err
		}
		if time.Since(sent) > 2*time.Minute {
			return ErrForbidden
		}
		change, e := tx.Model("biz_im_counter").Where("id", 1).Value("value")
		if e != nil {
			return e
		}
		if _, err = tx.Model("biz_im_message").Where("id", id).Update(g.Map{"status": "recalled", "content": "", "change_seq": change.Int64()}); err != nil {
			return err
		}
		if _, err = tx.Model("biz_im_conversation").Where("last_message_id", id).Update(g.Map{"last_content": "[message recalled]", "change_seq": change.Int64(), "updated_at": core.Now()}); err != nil {
			return err
		}
		event = g.Map{"id": id, "seq": row["seq"].Int64(), "stream_id": row["stream_id"].String()}
		return nil
	})
	if err == nil && event != nil {
		liveHub.publish(liveRecipients(ctx, recipients), g.Map{"type": "recall", "data": event})
	}
	return err
}

func bindControls(api *ghttp.RouterGroup) {
	api.POST("/groups/:group_id/action", func(r *ghttp.Request) {
		var in GroupAction
		if !parse(r, &in) {
			return
		}
		reply(r, nil, ChangeGroup(r.Context(), core.UID(r.Context()), r.GetRouter("group_id").String(), in))
	})
	api.POST("/friends/:user_id/action", func(r *ghttp.Request) {
		var in struct {
			Action      string `json:"action"`
			DisplayName string `json:"display_name"`
		}
		if !parse(r, &in) {
			return
		}
		reply(r, nil, ChangeFriend(r.Context(), core.UID(r.Context()), r.GetRouter("user_id").Int64(), in.Action, in.DisplayName))
	})
	api.POST("/messages/:id/recall", func(r *ghttp.Request) {
		reply(r, nil, RecallMessage(r.Context(), core.UID(r.Context()), r.GetRouter("id").String()))
	})
	api.GET("/messages/:id/receipts", func(r *ghttp.Request) {
		row, err := model(r.Context(), "biz_im_message").Where("id", r.GetRouter("id").String()).One()
		if err != nil {
			reply(r, nil, err)
			return
		}
		if len(row) == 0 {
			reply(r, nil, ErrInvalid)
			return
		}
		user := core.UID(r.Context())
		kind, target := row["conversation_type"].String(), row["target_id"].String()
		if kind == "private" && user != row["sender_id"].Int64() {
			target = strconv.FormatInt(row["sender_id"].Int64(), 10)
		}
		if _, err = historyParticipants(r.Context(), user, kind, target); err != nil {
			reply(r, nil, err)
			return
		}
		stream, err := streamID(kind, target, user)
		if err != nil || stream != row["stream_id"].String() {
			reply(r, nil, ErrForbidden)
			return
		}
		rows, err := model(r.Context(), "biz_im_conversation").Fields("user_id,last_read_seq,updated_at").Where("stream_id", row["stream_id"].String()).WhereGTE("last_read_seq", row["seq"].Int64()).All()
		replyProjected(r, rows, "receipt", err)
	})
}
