package im

import (
	"context"
	"errors"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

var ErrGone = errors.New("企业通讯录支持直接聊天，好友申请、审批和删除功能已停用")

func retiredFriends(r *ghttp.Request) { reply(r, nil, ErrGone) }
func contactAction(r *ghttp.Request) {
	var in struct {
		Action      string `json:"action"`
		DisplayName string `json:"display_name"`
	}
	if !parse(r, &in) {
		return
	}
	reply(r, true, contactData(r.Context(), core.UID(r.Context()), r.GetRouter("user_id").Int64(), in.Action, in.DisplayName))
}
func changeContact(ctx context.Context, user, peer int64, action, name string) error {
	if action == "delete" {
		return ErrGone
	}
	if user <= 0 || peer <= 0 || user == peer || len(name) > 160 {
		return ErrInvalid
	}
	if action != "block" && action != "unblock" && action != "update" {
		return ErrInvalid
	}
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		q := tx.Model("biz_im_friend").Where(g.Map{"user_id": user, "friend_user_id": peer})
		row, err := q.One()
		if err != nil {
			return err
		}
		data := g.Map{"updated_at": core.Now()}
		switch action {
		case "block":
			data["status"] = "blocked"
		case "unblock":
			data["status"] = "active"
		case "update":
			data["display_name"] = strings.TrimSpace(name)
		}
		if len(row) == 0 {
			data["id"] = guid.S()
			data["user_id"] = user
			data["friend_user_id"] = peer
			data["created_at"] = core.Now()
			if _, ok := data["status"]; !ok {
				data["status"] = "active"
			}
			if _, ok := data["display_name"]; !ok {
				data["display_name"] = ""
			}
			_, err = tx.Model("biz_im_friend").Data(data).Insert()
		} else {
			_, err = q.Update(data)
		}
		return err
	})
	if err == nil {
		liveHub.publish(liveRecipients(ctx, []int64{user}), g.Map{"type": "contact", "data": g.Map{"user_id": peer, "action": action}})
	}
	return err
}
