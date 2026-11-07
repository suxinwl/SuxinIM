package enterprise

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func isSignalUpgrade(r *http.Request) bool { return websocket.IsWebSocketUpgrade(r) }

// Rewrite only typed ICE URL fields for our published TURN ports. Credentials,
// SDP, fingerprints, metadata, peer IDs and unrelated external ICE stay intact.
func rewriteICEURL(raw, host string) string {
	colon := strings.IndexByte(raw, ':')
	if colon < 0 {
		return raw
	}
	scheme := raw[:colon]
	if scheme != "turn" && scheme != "turns" && scheme != "stun" {
		return raw
	}
	rest := raw[colon+1:]
	query := ""
	if index := strings.IndexByte(rest, '?'); index >= 0 {
		query = rest[index:]
		rest = rest[:index]
	}
	sourceHost, port, e := net.SplitHostPort(rest)
	if e != nil || port != "23478" && port != "25349" {
		return raw
	}
	lan, _ := url.Parse(envOrigin("PUBLIC_LAN_ORIGIN", "https://192.168.10.11:9885"))
	wan, _ := url.Parse(envOrigin("PUBLIC_INTERNET_ORIGIN", "https://im.szputy.com:9885"))
	if lan == nil || wan == nil || sourceHost != lan.Hostname() && sourceHost != wan.Hostname() {
		return raw
	}
	return scheme + ":" + net.JoinHostPort(host, port) + query
}
func rewriteICESignal(kind int, data []byte, host string) []byte {
	var signal livekit.SignalResponse
	if kind == websocket.BinaryMessage {
		if proto.Unmarshal(data, &signal) != nil {
			return data
		}
	} else if kind == websocket.TextMessage {
		if protojson.Unmarshal(data, &signal) != nil {
			return data
		}
	} else {
		return data
	}
	var servers []*livekit.ICEServer
	if join := signal.GetJoin(); join != nil {
		servers = join.IceServers
	}
	if reconnect := signal.GetReconnect(); reconnect != nil {
		servers = reconnect.IceServers
	}
	changed := false
	for _, server := range servers {
		for index, raw := range server.Urls {
			next := rewriteICEURL(raw, host)
			if next != raw {
				server.Urls[index] = next
				changed = true
			}
		}
	}
	if !changed {
		return data
	}
	var out []byte
	var e error
	if kind == websocket.BinaryMessage {
		out, e = proto.Marshal(&signal)
	} else {
		out, e = protojson.Marshal(&signal)
	}
	if e != nil {
		return data
	}
	return out
}
func (l *LiveKit) proxySignal(w http.ResponseWriter, r *http.Request) {
	origin := entryOrigin(r)
	profile, e := url.Parse(origin)
	if e != nil || profile.Hostname() == "" {
		writeError(w, forbidden("trusted media entry required"))
		return
	}
	target, e := url.Parse(l.Base)
	if e != nil {
		writeError(w, unavailable("media upstream unavailable"))
		return
	}
	target.Scheme = "ws"
	target.Path = r.URL.Path
	target.RawQuery = r.URL.RawQuery
	headers := http.Header{}
	headers.Set("Authorization", r.Header.Get("Authorization"))
	headers.Set("User-Agent", r.UserAgent())
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second, Subprotocols: websocket.Subprotocols(r)}
	upstream, resp, e := dialer.DialContext(r.Context(), target.String(), headers)
	if e != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		writeError(w, unavailable("media signaling unavailable"))
		return
	}
	defer upstream.Close()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, Subprotocols: websocket.Subprotocols(r)}
	client, e := upgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer client.Close()
	client.SetReadLimit(16 << 20)
	upstream.SetReadLimit(16 << 20)
	done := make(chan struct{}, 1)
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			kind, data, e := client.ReadMessage()
			if e != nil {
				return
			}
			if upstream.WriteMessage(kind, data) != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			kind, data, e := upstream.ReadMessage()
			if e != nil {
				return
			}
			data = rewriteICESignal(kind, data, profile.Hostname())
			if client.WriteMessage(kind, data) != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-r.Context().Done():
	}
}
