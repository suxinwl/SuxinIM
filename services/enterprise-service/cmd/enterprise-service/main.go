package main

import (
	"context"
	"log/slog"
	"net/http"
	"openim38.local/identity"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"openim38.local/enterprise-service/internal/enterprise"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func required(k string) string {
	v := os.Getenv(k)
	if strings.TrimSpace(v) == "" {
		slog.Error("missing required configuration", "name", k)
		os.Exit(2)
	}
	return v
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	dbctx, dbcancel := context.WithTimeout(ctx, 20*time.Second)
	defer dbcancel()
	client, err := mongo.Connect(dbctx, options.Client().ApplyURI(required("MONGO_URI")))
	if err != nil {
		fatal(err)
	}
	defer client.Disconnect(context.Background())
	if err = client.Ping(dbctx, nil); err != nil {
		fatal(err)
	}
	store, err := enterprise.NewMongoStore(dbctx, client, env("MONGO_DATABASE", "openim38_enterprise"))
	if err != nil {
		fatal(err)
	}
	auth, err := enterprise.NewGRPCAuth(env("CHAT_ADMIN_GRPC", "chat-admin:30200"), env("OPENIM_AUTH_GRPC", "openim-rpc-auth:10200"))
	if err != nil {
		fatal(err)
	}
	defer auth.Close()
	media, err := enterprise.NewLiveKit(env("LIVEKIT_HTTP_URL", "http://livekit:7880"), env("LIVEKIT_PUBLIC_URL", "wss://192.168.10.11:29880/livekit"), required("LIVEKIT_API_KEY"), required("LIVEKIT_API_SECRET"))
	if err != nil {
		fatal(err)
	}
	cipher, err := enterprise.NewSessionCipher(required("ENTERPRISE_SESSION_KEY"))
	if err != nil {
		fatal(err)
	}
	up := &enterprise.Upstream{IMBase: env("OPENIM_API", "http://openim-api:10002"), ChatBase: env("CHAT_API", "http://chat-api:10008"), ChatAdminBase: env("CHAT_ADMIN_API", "http://chat-admin-api:10009"), IMAdminID: env("OPENIM_ADMIN_ID", "imAdmin"), IMSecret: required("OPENIM_SECRET"), HTTP: &http.Client{Timeout: 60 * time.Second}}
	app := &enterprise.Server{Store: store, Auth: &enterprise.SessionAuth{Inner: auth, Store: store}, SessionRevoker: auth, IM: up, Upstream: up, Media: media, Presence: media, Catalog: &enterprise.GRPCCatalog{Auth: auth}, Cipher: cipher, WebhookSecret: required("WEBHOOK_SECRET")}
	server := &http.Server{Addr: env("LISTEN_ADDR", ":18090"), Handler: app, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	var bridgeServer *http.Server
	app.Messages = client.Database(env("OPENIM_MONGO_DATABASE", "openim38")).Collection("msg")
	if identity.Required() {
		bridge, err := enterprise.NewHRMBridge(app, auth)
		if err != nil {
			fatal(err)
		}
		app.HRM = bridge
		if err = bridge.State.Redis.Ping(dbctx).Err(); err != nil {
			fatal(err)
		}
		bridgeServer = &http.Server{Addr: env("HRM_BRIDGE_LISTEN", "127.0.0.1:10010"), Handler: bridge, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
		go func() {
			if err := bridgeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				fatal(err)
			}
		}()
	}
	go app.Run(ctx)
	go func() {
		<-ctx.Done()
		shutdown, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		_ = server.Shutdown(shutdown)
		if bridgeServer != nil {
			_ = bridgeServer.Shutdown(shutdown)
		}
	}()
	slog.Info("enterprise service starting", "listen", server.Addr, "database", env("MONGO_DATABASE", "openim38_enterprise"))
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatal(err)
	}
}
func fatal(err error) { slog.Error("enterprise service cannot start", "error", err); os.Exit(1) }
