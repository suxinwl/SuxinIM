package enterprise

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
)

type Receipt struct {
	ConversationID string           `json:"conversationID"`
	Message        Message          `json:"message"`
	Recipients     []string         `json:"recipientUserIDs"`
	Reads          map[string]int64 `json:"reads"`
	SnapshotTime   int64            `json:"snapshotTime"`
	Outbox         []pendingEvent   `json:"outbox"`
}

func receiptID(conversation, id string) string { return "receipt:" + conversation + ":" + id }
func (s *Server) snapshot(ctx context.Context, m Message) (Receipt, error) {
	conversation := "sg_" + m.GroupID
	id := receiptID(conversation, m.ClientMsgID)
	r, _, err := load[Receipt](ctx, s.Store, id)
	if err == nil {
		if r.Message.Seq == 0 && m.Seq > 0 {
			return mutate[Receipt](ctx, s.Store, id, func(r *Receipt) error {
				if r.Message.SendID != m.SendID {
					return bad("message identity changed")
				}
				r.Message = m
				return nil
			})
		}
		return r, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return r, err
	}
	us, err := s.IM.GroupMembers(ctx, m.GroupID)
	if err != nil {
		return r, err
	}
	recipients := []string{}
	sent := m.SendTime
	if sent == 0 {
		sent = s.now().UnixMilli()
	}
	for _, u := range us {
		joined := u.JoinTime
		if joined > 0 && joined < 100000000000 {
			joined *= 1000
		}
		if u.UserID != m.SendID && (joined == 0 || joined <= sent) {
			recipients = append(recipients, u.UserID)
		}
	}
	r = Receipt{ConversationID: conversation, Message: m, Recipients: unique(recipients), Reads: map[string]int64{}, SnapshotTime: s.now().UnixMilli()}
	err = s.Store.Save(ctx, document(id, "receipt", m.SendID, r), 0)
	if errors.Is(err, ErrConflict) {
		r, _, err = load[Receipt](ctx, s.Store, id)
	}
	return r, err
}
func (s *Server) receiptFor(ctx context.Context, a Actor, conversation, id string, seq int64, sender string) (Receipt, error) {
	if !strings.HasPrefix(conversation, "sg_") || strings.TrimPrefix(conversation, "sg_") == "" || id == "" {
		return Receipt{}, bad("group conversation and clientMsgID required")
	}
	r, _, err := load[Receipt](ctx, s.Store, receiptID(conversation, id))
	if errors.Is(err, ErrNotFound) || err == nil && r.Message.Seq == 0 {
		m, e := s.IM.Message(ctx, conversation, id, seq, sender)
		if e != nil {
			return r, e
		}
		r, err = s.snapshot(ctx, m)
	}
	if err != nil {
		return r, err
	}
	if sender != "" && sender != r.Message.SendID {
		return r, forbidden("supplied sender differs from real message")
	}
	if r.Message.SendID != a.UserID && !contains(r.Recipients, a.UserID) {
		return r, forbidden("not a message participant")
	}
	return r, nil
}
func (s *Server) receipts(ctx context.Context, a Actor, path string, q request) (any, error) {
	if err := checkActor(a, q.str("userID")); err != nil {
		return nil, err
	}
	conversation := q.str("conversationID")
	switch strings.TrimPrefix(path, "/msg/") {
	case "mark_group_message_read":
		var clients map[string]string
		if err := q.decode("clientMsgs", &clients); err != nil {
			return nil, err
		}
		if len(clients) == 0 || len(clients) > 200 {
			return nil, bad("clientMsgs count must be 1-200")
		}
		var seqs map[string]int64
		_ = q.decode("seqs", &seqs)
		mark := map[string]int{}
		for id, sender := range clients {
			old, err := s.receiptFor(ctx, a, conversation, id, seqs[id], sender)
			if err != nil {
				return nil, err
			}
			if old.Message.SendID == a.UserID {
				mark[id] = 0
				continue
			}
			changed := false
			r, err := mutate[Receipt](ctx, s.Store, receiptID(conversation, id), func(r *Receipt) error {
				changed = false
				if !contains(r.Recipients, a.UserID) {
					return forbidden("not a recipient at send time")
				}
				if _, ok := r.Reads[a.UserID]; ok {
					return nil
				}
				r.Reads[a.UserID] = s.now().UnixMilli()
				changed = true
				readMembers := []map[string]any{}
				for user, t := range r.Reads {
					readMembers = append(readMembers, map[string]any{"userID": user, "readTime": t})
				}
				r.Outbox = append(r.Outbox, pendingEvent{Key: receiptID(conversation, id) + ":" + a.UserID, Name: "OnGroupReadReceipt", Users: []string{r.Message.SendID}, Data: map[string]any{"conversationID": conversation, "groupMessageReadInfo": []any{map[string]any{"clientMsgID": id, "hasReadCount": len(r.Reads), "unreadCount": len(r.Recipients) - len(r.Reads), "readMembers": readMembers}}}})
				return nil
			})
			if err != nil {
				return nil, err
			}
			mark[id] = 0
			if changed {
				mark[id] = 1
			}
			if err = s.dispatchReceipt(ctx, r); err != nil {
				return nil, err
			}
		}
		return map[string]any{"mark": mark}, nil
	case "get_group_message_read_num":
		ids := q.strings("clientMsgIDs")
		if len(ids) == 0 {
			ids = q.strings("clientMsgIDList")
		}
		if len(ids) == 0 || len(ids) > 200 {
			return nil, bad("clientMsgIDs count must be 1-200")
		}
		var seqs map[string]int64
		_ = q.decode("seqs", &seqs)
		num := map[string]any{}
		for _, id := range ids {
			r, err := s.receiptFor(ctx, a, conversation, id, seqs[id], q.str("sendID"))
			if err != nil {
				return nil, err
			}
			num[id] = map[string]int{"readNum": len(r.Reads), "unreadNum": len(r.Recipients) - len(r.Reads)}
		}
		return map[string]any{"num": num}, nil
	case "get_group_message_has_read":
		filter := q.num("filter")
		if q.has("type") {
			filter = q.num("type")
		}
		if filter != 0 && filter != 1 {
			return nil, bad("filter must be 0 (read) or 1 (unread)")
		}
		r, err := s.receiptFor(ctx, a, conversation, q.str("clientMsgID"), q.num("seq"), q.str("sendID"))
		if err != nil {
			return nil, err
		}
		reads := []map[string]any{}
		for _, id := range r.Recipients {
			t, read := r.Reads[id]
			if (filter == 0 && read) || (filter == 1 && !read) {
				reads = append(reads, map[string]any{"userID": id, "readTime": t})
			}
		}
		sort.Slice(reads, func(i, j int) bool { return reads[i]["userID"].(string) < reads[j]["userID"].(string) })
		return map[string]any{"count": len(reads), "reads": page(reads, q), "recipientUserIDs": r.Recipients}, nil
	}
	return nil, missing("unknown receipt endpoint")
}
func (s *Server) dispatchReceipt(ctx context.Context, r Receipt) error {
	for _, e := range r.Outbox {
		if e.Delivered {
			continue
		}
		for _, u := range e.Users {
			if err := s.appendEvent(ctx, u, e.Key, e.Name, e.Data); err != nil {
				return err
			}
			s.queueHint(Actor{UserID: r.Message.SendID, Platform: 5}, u, e.Key, e.Name)
		}
		_, err := mutate[Receipt](ctx, s.Store, receiptID(r.ConversationID, r.Message.ClientMsgID), func(x *Receipt) error {
			pending := []pendingEvent{}
			for _, entry := range x.Outbox {
				if entry.Key != e.Key && !entry.Delivered {
					pending = append(pending, entry)
				}
			}
			x.Outbox = pending
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) reconcileReceipts(ctx context.Context) error {
	ds, err := s.Store.List(ctx, "receipt", "")
	if err != nil {
		return err
	}
	for _, d := range ds {
		var r Receipt
		if err = json.Unmarshal(d.Data, &r); err != nil {
			return err
		}
		if err = s.dispatchReceipt(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) webhook(w http.ResponseWriter, r *http.Request, path string) {
	prefix := "/internal/openim/webhook/"
	tail := strings.TrimPrefix(path, prefix)
	parts := strings.Split(tail, "/")
	if !strings.HasPrefix(path, prefix) || len(parts) < 1 || s.WebhookSecret == "" || subtle.ConstantTimeCompare([]byte(parts[0]), []byte(s.WebhookSecret)) != 1 {
		writeError(w, unauthorized("invalid webhook credential"))
		return
	}
	if r.Method != "POST" {
		writeError(w, bad("POST required"))
		return
	}
	q, err := parseRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	command := q.str("callbackCommand")
	if len(parts) > 1 && parts[1] != "" && command != parts[1] {
		writeError(w, bad("callback command differs from URL"))
		return
	}
	if command == "callbackBeforeSendGroupMsgCommand" || command == "callbackAfterSendGroupMsgCommand" || command == "callbackAfterMsgSaveDBCommand" {
		if q.str("groupID") != "" && q.str("clientMsgID") != "" && q.str("sendID") != "" && (q.num("sessionType") == 3 || q.num("sessionType") == 0) {
			m := Message{ClientMsgID: q.str("clientMsgID"), SendID: q.str("sendID"), GroupID: q.str("groupID"), Seq: q.num("seq"), SendTime: q.num("sendTime"), ContentType: int(q.num("contentType"))}
			_, err = s.snapshot(r.Context(), m)
			if err != nil {
				writeError(w, err)
				return
			}
		}
	}
	// Store each delivery once for diagnostics. Message snapshots themselves are
	// keyed by real conversation/clientMsgID, so callback order cannot duplicate it.
	// Some native callbacks (notably single-message read) have no operationID
	// or serverMsgID. UserID alone would collapse every later read by that user.
	// Canonical map encoding keeps exact retries idempotent while preserving
	// different conversations, sequences, recipients and connection events.
	identity := command + ":" + tokenHash(string(mustJSON(q)))
	err = s.Store.Save(r.Context(), document("webhook:"+identity, "webhook", command, q), 0)
	if err != nil && !errors.Is(err, ErrConflict) {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"actionCode": 0, "errCode": 0, "errMsg": "", "errDlt": "", "nextCode": 0})
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
