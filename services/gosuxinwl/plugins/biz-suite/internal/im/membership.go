package im

import (
	"context"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/framework/util/guid"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"strings"
)

func CreateSpace(ctx context.Context, user int64, kind, name, description string, approval int) (g.Map, error) {
	if err := communicationAccount(ctx, user, true); err != nil {
		return nil, err
	}
	if user <= 0 || (kind != "group" && kind != "channel") || strings.TrimSpace(name) == "" || len(name) > 160 || len(description) > 500 {
		return nil, ErrInvalid
	}
	stamp := core.Now()
	id := guid.S()
	data := g.Map{"id": id, "owner_id": user, "name": name, "status": "active", "created_at": stamp, "updated_at": stamp}
	if kind == "group" {
		data["notice"] = description
		data["join_approval"] = approval
	} else {
		data["description"] = description
	}
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, err := tx.Model("biz_im_" + kind).Data(data).Insert(); err != nil {
			return err
		}
		_, err := tx.Model("biz_im_" + kind + "_member").Data(g.Map{"id": guid.S(), kind + "_id": id, "user_id": user, "role": "owner", "status": "active", "joined_at": stamp}).Insert()
		return err
	})
	return data, err
}

func manager(ctx context.Context, user int64, kind, id string) error {
	if _, err := participants(ctx, user, kind, id); err != nil {
		return err
	}
	n, err := model(ctx, "biz_im_"+kind+"_member").Where(g.Map{kind + "_id": id, "user_id": user, "status": "active"}).WhereIn("role", []string{"owner", "admin"}).Count()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrForbidden
	}
	return nil
}

func JoinSpace(ctx context.Context, user int64, kind, id string) (g.Map, error) {
	if err := communicationAccount(ctx, user, true); err != nil {
		return nil, err
	}
	if kind != "group" && kind != "channel" {
		return nil, ErrInvalid
	}
	if user <= 0 {
		return nil, ErrForbidden
	}
	var out g.Map
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		space, e := tx.Model("biz_im_" + kind).Where(g.Map{"id": id, "status": "active"}).One()
		if e != nil {
			return e
		}
		if len(space) == 0 {
			return ErrForbidden
		}
		member, e := tx.Model("biz_im_" + kind + "_member").Where(g.Map{kind + "_id": id, "user_id": user}).One()
		if e != nil {
			return e
		}
		if len(member) > 0 && member["status"].String() == "active" {
			out = member.Map()
			return nil
		}
		stamp := core.Now()
		if kind == "group" && space["join_approval"].Int() != 0 {
			q := tx.Model("biz_im_group_request").Where(g.Map{"group_id": id, "user_id": user})
			row, e := q.One()
			if e != nil {
				return e
			}
			out = g.Map{"status": "pending", "updated_at": stamp}
			if len(row) == 0 {
				out["id"] = guid.S()
				out["group_id"] = id
				out["user_id"] = user
				out["created_at"] = stamp
				_, e = tx.Model("biz_im_group_request").Data(out).Insert()
			} else {
				out["id"] = row["id"].String()
				_, e = q.Update(out)
			}
			return e
		}
		out = g.Map{"id": guid.S(), kind + "_id": id, "user_id": user, "role": "member", "status": "active", "joined_at": stamp}
		if len(member) > 0 {
			out["id"] = member["id"].String()
			_, e = tx.Model("biz_im_"+kind+"_member").Where("id", out["id"]).Update(out)
		} else {
			_, e = tx.Model("biz_im_" + kind + "_member").Data(out).Insert()
		}
		return e
	})
	return out, err
}

func ReviewGroupRequest(ctx context.Context, user int64, groupID, requestID string, approve bool) error {
	if err := currentAccount(ctx, user, true); err != nil {
		return err
	}
	if err := manager(ctx, user, "group", groupID); err != nil {
		return err
	}
	var memberID int64
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		if err := manager(ctx, user, "group", groupID); err != nil {
			return err
		}
		q := tx.Model("biz_im_group_request").Where(g.Map{"id": requestID, "group_id": groupID})
		row, e := q.One()
		if e != nil {
			return e
		}
		if len(row) == 0 {
			return ErrInvalid
		}
		memberID = row["user_id"].Int64()
		if approve {
			if err := communicationAccount(ctx, memberID, false); err != nil {
				return err
			}
		}
		status := "rejected"
		if approve {
			status = "approved"
		}
		if row["status"].String() != "pending" {
			if row["status"].String() == status {
				return nil
			}
			return ErrConflict
		}
		if _, e = q.Update(g.Map{"status": status, "updated_at": core.Now()}); e != nil {
			return e
		}
		if !approve {
			return nil
		}
		member := tx.Model("biz_im_group_member").Where(g.Map{"group_id": groupID, "user_id": memberID})
		n, e := member.Count()
		if e != nil {
			return e
		}
		if n > 0 {
			_, e = member.Update(g.Map{"status": "active", "role": "member", "muted": 0, "joined_at": core.Now()})
		} else {
			_, e = tx.Model("biz_im_group_member").Data(g.Map{"id": guid.S(), "group_id": groupID, "user_id": memberID, "role": "member", "status": "active", "joined_at": core.Now()}).Insert()
		}
		return e
	})
	if err == nil {
		liveHub.publish(liveRecipients(ctx, []int64{memberID}), g.Map{"type": "membership", "data": g.Map{"group_id": groupID, "approved": approve}})
	}
	return err
}

func RequestFriend(ctx context.Context, user, peer int64, message string) (g.Map, error) {
	if user <= 0 || peer <= 0 || peer == user || len(message) > 500 {
		return nil, ErrInvalid
	}
	exists, err := core.HostUserExists(ctx, peer)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrForbidden
	}
	var out g.Map
	err = core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		q := tx.Model("biz_im_friend_request").Where(g.Map{"sender_id": user, "receiver_id": peer})
		old, e := q.One()
		if e != nil {
			return e
		}
		out = g.Map{"sender_id": user, "receiver_id": peer, "message": message, "status": "pending", "updated_at": core.Now()}
		if len(old) > 0 {
			out["id"] = old["id"].String()
			if old["status"].String() == "approved" {
				out = old.Map()
				return nil
			}
			_, e = q.Update(out)
		} else {
			out["id"] = guid.S()
			out["created_at"] = core.Now()
			_, e = tx.Model("biz_im_friend_request").Data(out).Insert()
		}
		return e
	})
	if err == nil {
		liveHub.publish([]int64{peer}, g.Map{"type": "friend_request", "data": out})
	}
	return out, err
}

func ReviewFriendRequest(ctx context.Context, user int64, id string, approve bool) error {
	var peer int64
	err := core.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := lockCounter(ctx, tx); err != nil {
			return err
		}
		q := tx.Model("biz_im_friend_request").Where("id", id)
		row, e := q.One()
		if e != nil {
			return e
		}
		if len(row) == 0 || row["receiver_id"].Int64() != user {
			return ErrForbidden
		}
		peer = row["sender_id"].Int64()
		status := "rejected"
		if approve {
			status = "approved"
		}
		if row["status"].String() != "pending" {
			if row["status"].String() == status {
				return nil
			}
			return ErrConflict
		}
		if _, e = q.Update(g.Map{"status": status, "updated_at": core.Now()}); e != nil {
			return e
		}
		if !approve {
			return nil
		}
		for _, pair := range [][2]int64{{user, peer}, {peer, user}} {
			q := tx.Model("biz_im_friend").Where(g.Map{"user_id": pair[0], "friend_user_id": pair[1]})
			n, e := q.Count()
			if e != nil {
				return e
			}
			if n == 0 {
				_, e = tx.Model("biz_im_friend").Data(g.Map{"id": guid.S(), "user_id": pair[0], "friend_user_id": pair[1], "display_name": "", "status": "active", "created_at": core.Now(), "updated_at": core.Now()}).Insert()
			} else {
				// Legacy internal import helpers must never clear a personal block.
				_, e = q.WhereNot("status", "blocked").Update(g.Map{"status": "active", "updated_at": core.Now()})
			}
			if e != nil {
				return e
			}
		}
		return nil
	})
	if err == nil {
		liveHub.publish([]int64{peer, user}, g.Map{"type": "friend_request", "data": g.Map{"id": id, "approved": approve}})
	}
	return err
}
