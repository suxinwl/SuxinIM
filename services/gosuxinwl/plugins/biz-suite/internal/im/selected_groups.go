package im

import (
	"context"
	"strings"

	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

const maxSelectedMembers = 200

// Selected colleagues are invited by the creator, so join_approval continues
// to govern later self-service requests, not this initial membership list.
func CreateGroupWithMembers(ctx context.Context, user int64, name, description string, approval int, members []int64) (g.Map, error) {
	if err := currentAccount(ctx, user, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" || len(name) > 160 || len(description) > 500 || (approval != 0 && approval != 1) || len(members) > maxSelectedMembers {
		return nil, ErrInvalid
	}
	ids := []int64{user}
	seen := map[int64]bool{user: true}
	for _, id := range members {
		if id <= 0 {
			return nil, ErrInvalid
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if len(ids) < 2 {
		return nil, ErrInvalid
	}
	validate := func(ctx context.Context) error {
		d, err := loadDirectory(ctx)
		if err != nil {
			return err
		}
		for _, id := range ids {
			p := d.byUser[id]
			if p == nil || !p.active || !p.read || (id == user && !p.write) {
				return ErrForbidden
			}
		}
		return nil
	}
	if err := validate(ctx); err != nil {
		return nil, err
	}
	id, stamp := guid.S(), core.Now()
	out := g.Map{"id": id, "owner_id": user, "name": strings.TrimSpace(name), "notice": description, "join_approval": approval, "status": "active", "created_at": stamp, "updated_at": stamp}
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		if err := validate(ctx); err != nil {
			return err
		}
		if _, err := tx.Model("biz_im_group").Data(out).Insert(); err != nil {
			return err
		}
		seq, err := tx.Model("biz_im_counter").Where("id", 1).Value("value")
		if err != nil {
			return err
		}
		for _, uid := range ids {
			role := "member"
			if uid == user {
				role = "owner"
			}
			if _, err = tx.Model("biz_im_group_member").Data(g.Map{"id": guid.S(), "group_id": id, "user_id": uid, "role": role, "status": "active", "joined_at": stamp}).Insert(); err != nil {
				return err
			}
			if _, err = tx.Model("biz_im_conversation").Data(g.Map{"id": guid.S(), "user_id": uid, "conversation_type": "group", "conversation_id": id, "stream_id": "group:" + id, "change_seq": seq.Int64(), "last_seq": 0, "last_read_seq": 0, "unread_count": 0, "last_message_id": "", "last_content": "", "updated_at": stamp}).Insert(); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		liveHub.publish(liveRecipients(ctx, ids), g.Map{"type": "membership", "data": g.Map{"group_id": id, "action": "created"}})
	}
	return out, err
}
