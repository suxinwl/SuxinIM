package im

import (
	"context"
	"fmt"
	"github.com/suxinwl/GoSuxin/framework/database/gdb"
	"strconv"

	"github.com/suxinwl/GoSuxin/framework/frame/g"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
)

type historicalImportKey struct{}

func isHistoricalImport(ctx context.Context) bool {
	v, _ := ctx.Value(historicalImportKey{}).(bool)
	return v
}

func currentAccount(ctx context.Context, user int64, write bool) error {
	if user <= 0 {
		return ErrForbidden
	}
	ok, err := core.HostModel("admin").Ctx(ctx).Where("id", user).Where("status", 0).WhereNull("deletetime").Exist()
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	grants, err := accountGrants(ctx)
	if err != nil {
		return err
	}
	p := grants[user]
	if !p.gate || !p.read || (write && !p.write) {
		return ErrForbidden
	}
	return nil
}
func privateCommunication(ctx context.Context, user, peer int64, write bool) error {
	if _, err := streamID("private", strconv.FormatInt(peer, 10), user); err != nil {
		return err
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	a, b := d.byUser[user], d.byUser[peer]
	if a == nil || b == nil || !a.active || !a.read || !b.active || !b.read || (write && !a.write) {
		return ErrForbidden
	}
	blocked, err := model(ctx, "biz_im_friend").Where("((user_id = ? AND friend_user_id = ?) OR (user_id = ? AND friend_user_id = ?)) AND status = ?", user, peer, peer, user, "blocked").Exist()
	if err != nil {
		return err
	}
	if blocked {
		return ErrForbidden
	}
	return nil
}
func historyParticipants(ctx context.Context, user int64, kind, target string) ([]int64, error) {
	if kind != "private" {
		return participants(ctx, user, kind, target)
	}
	if _, err := streamID(kind, target, user); err != nil {
		return nil, err
	}
	if err := currentAccount(ctx, user, false); err != nil {
		return nil, err
	}
	peer, _ := strconv.ParseInt(target, 10, 64)
	return []int64{user, peer}, nil
}
func hasPrivateHistory(ctx context.Context, user, peer int64) bool {
	stream, err := streamID("private", strconv.FormatInt(peer, 10), user)
	if err != nil {
		return false
	}
	exists, err := model(ctx, "biz_im_message").Where("stream_id", stream).Exist()
	return err == nil && exists
}
func historicalPrivateParticipants(ctx context.Context, user int64, target string) ([]int64, error) {
	if _, err := streamID("private", target, user); err != nil {
		return nil, err
	}
	peer, _ := strconv.ParseInt(target, 10, 64)
	n, err := core.HostModel("admin").Ctx(ctx).WhereIn("id", []int64{user, peer}).WhereNull("deletetime").Count()
	if err != nil {
		return nil, err
	}
	if n != 2 {
		return nil, ErrForbidden
	}
	return []int64{user, peer}, nil
}

// Keep only recipients that still have an enabled host identity, plugin gate
// and IM read access. Membership alone cannot preserve a revoked socket.
func liveRecipients(ctx context.Context, users []int64) []int64 {
	if len(users) == 0 {
		return []int64{}
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return []int64{}
	}
	out := []int64{}
	for _, id := range users {
		p := d.byUser[id]
		if p != nil && p.active && p.read {
			out = append(out, id)
		}
	}
	return out
}

func contactData(ctx context.Context, user, peer int64, action, name string) error {
	if action == "delete" {
		return ErrGone
	}
	if err := currentAccount(ctx, user, true); err != nil {
		return err
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	if (d.byUser[peer] == nil || !d.byUser[peer].listed) && !hasPrivateHistory(ctx, user, peer) {
		return ErrForbidden
	}
	return changeContact(ctx, user, peer, action, name)
}

func decorateRows(ctx context.Context, rows []g.Map, kind string) error {
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	if err = d.personalized(ctx, core.UID(ctx)); err != nil {
		return err
	}
	for _, row := range rows {
		var uid int64
		switch kind {
		case "message":
			uid = number(row["sender_id"])
		case "member":
			uid = number(row["user_id"])
		case "receipt":
			uid = number(row["user_id"])
		case "space":
			uid = number(row["owner_id"])
		case "conversation":
			if row["conversation_type"] == "private" {
				uid = number(row["conversation_id"])
			}
		}
		p := d.byUser[uid]
		name, no, avatar := "账号已删除", "", ""
		if p != nil {
			name, no, avatar = p.Name, p.EmployeeNo, p.Avatar
			if name == "" {
				name = "未提供姓名"
			}
		}
		if kind == "message" {
			delete(row, "request_hash")
			row["sender_name"], row["sender_employee_no"], row["sender_avatar"] = name, no, avatar
		} else if kind == "space" {
			row["owner_name"], row["owner_employee_no"] = name, no
		} else if uid > 0 {
			row["name"], row["employee_no"], row["avatar"] = name, no, avatar
			if kind == "receipt" {
				row["status"], row["read_at"] = "read", row["updated_at"]
			}
			if kind == "conversation" {
				row["can_chat"], row["unavailable_reason"], row["display_name"] = false, "账号已删除", ""
				if p != nil {
					row["can_chat"], row["unavailable_reason"], row["display_name"], row["blocked_by_me"] = p.CanChat, p.UnavailableReason, p.DisplayName, p.BlockedByMe
					row["can_send"], row["can_read"], row["can_call"], row["readonly_reason"] = p.CanSend, p.CanRead, p.CanCall, p.ReadonlyReason
				}
			}
		}
	}
	if kind == "conversation" {
		for _, space := range []string{"group", "channel"} {
			spaces, e := model(ctx, "biz_im_"+space).Fields("id,name").All()
			if e != nil {
				return e
			}
			names := map[string]string{}
			for _, r := range spaces {
				names[r["id"].String()] = r["name"].String()
			}
			for _, row := range rows {
				if row["conversation_type"] == space {
					row["name"] = names[fmtString(row["conversation_id"])]
					row["can_chat"] = true
				}
			}
		}
	}
	return nil
}
func communicationAccount(ctx context.Context, user int64, write bool) error {
	d, err := loadDirectory(ctx)
	if err != nil {
		return err
	}
	p := d.byUser[user]
	if p == nil || !p.active || !p.read || (write && !p.write) {
		return ErrForbidden
	}
	return nil
}

// Filter before limiting, otherwise inaccessible old group rows can starve a
// later legitimate private conversation in an incremental 200-row page.
func visibleConversations(ctx context.Context, user, after int64) (gdb.Result, error) {
	rows, err := model(ctx, "biz_im_conversation").Where("user_id", user).WhereGT("change_seq", after).OrderAsc("change_seq").All()
	if err != nil {
		return nil, err
	}
	d, err := loadDirectory(ctx)
	if err != nil {
		return nil, err
	}
	allowed := map[string]bool{}
	me := d.byUser[user]
	if me != nil && me.active && me.read {
		for _, kind := range []string{"group", "channel"} {
			ids, e := model(ctx, "biz_im_"+kind+"_member").Where(g.Map{"user_id": user, "status": "active"}).Array(kind + "_id")
			if e != nil {
				return nil, e
			}
			if len(ids) == 0 {
				continue
			}
			spaces, e := model(ctx, "biz_im_"+kind).WhereIn("id", ids).Where("status", "active").Array("id")
			if e != nil {
				return nil, e
			}
			for _, id := range spaces {
				allowed[kind+":"+id.String()] = true
			}
		}
	}
	out := gdb.Result{}
	for _, row := range rows {
		if row["conversation_type"].String() == "private" || allowed[row["stream_id"].String()] {
			out = append(out, row)
			if len(out) == 200 {
				break
			}
		}
	}
	return out, nil
}
func number(v any) int64     { n, _ := strconv.ParseInt(fmtString(v), 10, 64); return n }
func fmtString(v any) string { return fmt.Sprint(v) }
