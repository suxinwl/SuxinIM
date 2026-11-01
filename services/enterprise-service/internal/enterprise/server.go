package enterprise

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Store               Store
	Auth                Authenticator
	IM                  IM
	Upstream            *Upstream
	Media               Media
	Presence            MediaPresence
	SessionRevoker      SessionRevoker
	Catalog             Catalog
	Cipher              *sessionCipher
	WebhookSecret       string
	Now                 func() time.Time
	lastSessionCheck    time.Time
	lastPresenceCheck   time.Time
	lastRevocationCheck time.Time
	hintOnce            sync.Once
	hintQueue           chan eventHint
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}
func newID() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

type request map[string]json.RawMessage

func (q request) str(k string) string { var s string; _ = json.Unmarshal(q[k], &s); return s }
func (q request) num(k string) int64 {
	var n json.Number
	if json.Unmarshal(q[k], &n) == nil {
		x, _ := n.Int64()
		return x
	}
	return 0
}
func (q request) boolean(k string) bool { var v bool; _ = json.Unmarshal(q[k], &v); return v }
func (q request) strings(k string) []string {
	var s []string
	_ = json.Unmarshal(q[k], &s)
	return unique(s)
}
func (q request) has(k string) bool { _, ok := q[k]; return ok }
func (q request) decode(k string, v any) error {
	if len(q[k]) == 0 {
		return bad(k + " required")
	}
	if e := json.Unmarshal(q[k], v); e != nil {
		return bad(k + " invalid")
	}
	return nil
}
func parseRequest(r *http.Request) (request, error) {
	q := request{}
	if r.Method == "GET" {
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				q[k], _ = json.Marshal(v[0])
				if _, e := strconv.ParseInt(v[0], 10, 64); e == nil {
					q[k] = json.RawMessage(v[0])
				}
			}
		}
		return q, nil
	}
	d := json.NewDecoder(io.LimitReader(r.Body, 2<<20))
	if err := d.Decode(&q); err != nil && err != io.EOF {
		return nil, bad("invalid JSON")
	}
	return q, nil
}
func unique(ss []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range ss {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
func contains(ss []string, s string) bool {
	for _, v := range ss {
		if s == v {
			return true
		}
	}
	return false
}
func without(ss []string, remove ...string) []string {
	out := []string{}
	for _, v := range ss {
		if !contains(remove, v) {
			out = append(out, v)
		}
	}
	return unique(out)
}
func pagination(q request) (int, int) {
	offset, count := int(q.num("offset")), int(q.num("count"))
	if q.has("pagination") {
		var p struct {
			PageNumber int `json:"pageNumber"`
			ShowNumber int `json:"showNumber"`
		}
		_ = q.decode("pagination", &p)
		count = p.ShowNumber
		if p.PageNumber > 0 {
			offset = (p.PageNumber - 1) * count
		}
	}
	if count <= 0 {
		count = 20
	}
	if count > 200 {
		count = 200
	}
	if offset < 0 {
		offset = 0
	}
	return offset, count
}
func page[T any](xs []T, q request) []T {
	o, n := pagination(q)
	if o >= len(xs) {
		return []T{}
	}
	if o+n > len(xs) {
		n = len(xs) - o
	}
	return xs[o : o+n]
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errCode": 0, "errMsg": "", "errDlt": "", "data": data})
}
func writeError(w http.ResponseWriter, err error) {
	var a *apiError
	if !errors.As(err, &a) {
		switch {
		case errors.Is(err, ErrNotFound):
			a = &apiError{404, 1004, "record not found"}
		case errors.Is(err, ErrConflict):
			a = &apiError{409, 1005, "record changed; retry"}
		default:
			slog.Error("enterprise request failed", "error", err)
			a = &apiError{503, 1006, "service temporarily unavailable"}
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(a.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"errCode": a.Code, "errMsg": a.Message, "errDlt": "", "data": nil})
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/enterprise/v1")
	if path == "/healthz" {
		writeJSON(w, 200, map[string]any{"service": "enterprise-service", "status": "alive"})
		return
	}
	if strings.HasPrefix(path, "/livekit/") {
		s.mediaGateway(w, r)
		return
	}
	if path == "/readyz" {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := s.Store.Ping(ctx); err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"status": "ready"})
		return
	}
	if path == "/internal/livekit/webhook" {
		s.livekitWebhook(w, r)
		return
	}
	if strings.HasPrefix(path, "/internal/") {
		s.webhook(w, r, path)
		return
	}
	if s.adminRoutes(w, r, path) {
		return
	}
	if path == "/session/logout" {
		s.sessionLogout(w, r)
		return
	}
	if r.Method != "POST" && !(r.Method == "GET" && path == "/rtc/events") {
		writeError(w, &apiError{405, 1001, "method not allowed"})
		return
	}
	q, err := parseRequest(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if path == "/application/latest" {
		v, e := s.latest(r.Context(), q)
		if e != nil {
			writeError(w, e)
		} else {
			writeJSON(w, 200, v)
		}
		return
	}
	adminOnly := strings.HasPrefix(path, "/application/publish") || strings.HasPrefix(path, "/admin/")
	a, err := s.Auth.Authenticate(r.Context(), r, adminOnly)
	if err != nil {
		writeError(w, err)
		return
	}
	var data any
	switch {
	case strings.HasPrefix(path, "/office/"):
		data, err = s.office(r.Context(), a, path, q)
	case strings.HasPrefix(path, "/msg/"):
		data, err = s.receipts(r.Context(), a, path, q)
	case strings.HasPrefix(path, "/signaling/"):
		data, err = s.signaling(r.Context(), a, path, q)
	case path == "/rtc/events":
		data, err = s.events(r.Context(), a, q)
	case strings.HasPrefix(path, "/rtc/"):
		data, err = s.records(r.Context(), a, path, q)
	case strings.HasPrefix(path, "/admin/rtc/"):
		data, err = s.records(r.Context(), a, strings.TrimPrefix(path, "/admin"), q)
	case path == "/application/publish":
		data, err = s.publish(r.Context(), a, q)
	default:
		err = missing("unknown enterprise endpoint")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, 200, data)
}
func (s *Server) Run(ctx context.Context) {
	s.initHints()
	for i := 0; i < 4; i++ {
		go s.hintWorker(ctx)
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.retrySessionRevocations(ctx); err != nil {
				slog.Error("session revocation retry failed", "error", err)
			}
			if err := s.reconcileRooms(ctx); err != nil {
				slog.Error("room reconciliation failed", "error", err)
			}
			if err := s.reconcileMediaPresence(ctx); err != nil {
				slog.Error("media presence reconciliation failed", "error", err)
			}
			if err := s.reconcileReceipts(ctx); err != nil {
				slog.Error("receipt reconciliation failed", "error", err)
			}
		}
	}
}

type eventHint struct {
	Sender          Actor
	User, Key, Name string
}
type IMHints interface {
	Hint(context.Context, Actor, string, string) error
}

func (s *Server) initHints() { s.hintOnce.Do(func() { s.hintQueue = make(chan eventHint, 256) }) }
func (s *Server) queueHint(a Actor, user, key, name string) {
	if _, ok := s.IM.(IMHints); !ok {
		return
	}
	s.initHints()
	select {
	case s.hintQueue <- eventHint{a, user, key, name}:
	default:
	}
}
func (s *Server) hintWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case h := <-s.hintQueue:
			hint, ok := s.IM.(IMHints)
			if !ok {
				continue
			}
			payload := string(mustJSON(map[string]any{"customType": 901, "enterpriseEvent": true, "eventID": h.Key, "event": h.Name, "kind": "enterprise_event"}))
			bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
			err := hint.Hint(bounded, h.Sender, h.User, payload)
			cancel()
			if err != nil {
				slog.Debug("event hint skipped; client polling recovers state", "error", err)
			}
		}
	}
}
func sortedDocs(ds []Document) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Updated.Equal(ds[j].Updated) {
			return ds[i].ID < ds[j].ID
		}
		return ds[i].Updated.After(ds[j].Updated)
	})
}

type Event struct {
	ID    int64  `json:"id"`
	Event string `json:"event"`
	Data  any    `json:"data"`
	Key   string `json:"eventKey,omitempty"`
}
type eventStream struct {
	Cursor int64           `json:"cursor"`
	Floor  int64           `json:"floor"`
	Events []Event         `json:"events"`
	Seen   map[string]bool `json:"seen"`
}

func (s *Server) appendEvent(ctx context.Context, user, key, name string, data any) error {
	id := "events:" + user
	for i := 0; i < 16; i++ {
		stream, d, err := load[eventStream](ctx, s.Store, id)
		if errors.Is(err, ErrNotFound) {
			stream = eventStream{Events: []Event{}, Seen: map[string]bool{}}
			d = document(id, "events", user, stream)
		} else if err != nil {
			return err
		}
		if stream.Seen == nil {
			stream.Seen = map[string]bool{}
		}
		if stream.Seen[key] {
			return nil
		}
		stream.Cursor++
		stream.Seen[key] = true
		stream.Events = append(stream.Events, Event{ID: stream.Cursor, Event: name, Data: data, Key: key})
		if len(stream.Events) > 2000 {
			stream.Floor = stream.Events[0].ID
			delete(stream.Seen, stream.Events[0].Key)
			stream.Events = stream.Events[1:]
		}
		d.Data, _ = json.Marshal(stream)
		for len(d.Data) > 4<<20 && len(stream.Events) > 1 {
			stream.Floor = stream.Events[0].ID
			delete(stream.Seen, stream.Events[0].Key)
			stream.Events = stream.Events[1:]
			d.Data, _ = json.Marshal(stream)
		}
		if len(d.Data) > 4<<20 {
			return bad("event snapshot exceeds 4 MiB")
		}
		if err = s.Store.Save(ctx, d, d.Version); errors.Is(err, ErrConflict) {
			continue
		} else {
			return err
		}
	}
	return ErrConflict
}
func (s *Server) events(ctx context.Context, a Actor, q request) (any, error) {
	stream, _, err := load[eventStream](ctx, s.Store, "events:"+a.UserID)
	if errors.Is(err, ErrNotFound) {
		return map[string]any{"events": []Event{}, "cursor": 0, "resyncRequired": false}, nil
	}
	if err != nil {
		return nil, err
	}
	after := q.num("after")
	if after < 0 || q.num("bootstrap") == 1 {
		return map[string]any{"events": []Event{}, "cursor": stream.Cursor, "resyncRequired": false}, nil
	}
	if after > stream.Cursor {
		return map[string]any{"events": []Event{}, "cursor": stream.Cursor, "resyncRequired": true}, nil
	}
	events := []Event{}
	cursor := after
	for _, e := range stream.Events {
		if e.ID > after {
			events = append(events, e)
			cursor = e.ID
			if len(events) == 200 {
				break
			}
		}
	}
	if cursor < stream.Floor {
		cursor = stream.Cursor
	}
	return map[string]any{"events": events, "cursor": cursor, "resyncRequired": after < stream.Floor}, nil
}
