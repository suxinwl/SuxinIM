package im

import (
	"context"
	"encoding/json"
	ws "github.com/gorilla/websocket"
	"github.com/suxinwl/GoSuxin/framework/net/ghttp"
	"github.com/suxinwl/GoSuxin/plugins/biz-suite/internal/core"
	"sync"
	"time"
)

type subscription struct {
	events chan []byte
	done   chan struct{}
	once   sync.Once
}

func (s *subscription) close() { s.once.Do(func() { close(s.done) }) }

type hub struct {
	mu    sync.RWMutex
	users map[int64]map[*subscription]bool
}

var liveHub = &hub{users: make(map[int64]map[*subscription]bool)}

func (h *hub) subscribe(user int64) *subscription {
	s := &subscription{events: make(chan []byte, 128), done: make(chan struct{})}
	h.mu.Lock()
	if h.users[user] == nil {
		h.users[user] = make(map[*subscription]bool)
	}
	h.users[user][s] = true
	h.mu.Unlock()
	return s
}
func (h *hub) unsubscribe(user int64, s *subscription) {
	h.mu.Lock()
	delete(h.users[user], s)
	if len(h.users[user]) == 0 {
		delete(h.users, user)
	}
	h.mu.Unlock()
	s.close()
}
func (h *hub) publish(users []int64, event any) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := map[int64]bool{}
	for _, id := range users {
		if seen[id] {
			continue
		}
		seen[id] = true
		for s := range h.users[id] {
			select {
			case <-s.done:
			case s.events <- data:
			default:
				s.close()
			}
		}
	}
}

// WebSocket only transports committed server events. Mutations use REST so
// retries, authorization and idempotency have one implementation. Slow clients
// disconnect and catch up from their persisted sequence over REST.
func websocket(r *ghttp.Request) {
	serveWebSocket(r, 25*time.Second)
}

func socketAuthorized(ctx context.Context, user int64) bool {
	check, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return communicationAccount(check, user, false) == nil
}

func serveWebSocket(r *ghttp.Request, heartbeat time.Duration) {
	user := core.UID(r.Context())
	if user <= 0 {
		r.Response.WriteStatus(401)
		return
	}
	if !socketAuthorized(r.Context(), user) {
		r.Response.WriteStatus(403)
		return
	}
	upgrader := ws.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 4096}
	conn, err := upgrader.Upgrade(r.Response.Writer, r.Request, nil)
	if err != nil {
		return
	}
	s := liveHub.subscribe(user)
	defer liveHub.unsubscribe(user, s)
	defer conn.Close()
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(70 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(70 * time.Second)) })
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer conn.Close()
		ticker := time.NewTicker(heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case event := <-s.events:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if conn.WriteMessage(ws.TextMessage, event) != nil {
					return
				}
			case <-ticker.C:
				if !socketAuthorized(r.Context(), user) {
					_ = conn.WriteControl(ws.CloseMessage, ws.FormatCloseMessage(ws.ClosePolicyViolation, "IM permission revoked"), time.Now().Add(time.Second))
					return
				}
				if conn.WriteControl(ws.PingMessage, nil, time.Now().Add(10*time.Second)) != nil {
					return
				}
			}
		}
	}()
	for {
		if _, _, err = conn.ReadMessage(); err != nil {
			break
		}
	}
	s.close()
	<-writerDone
}
