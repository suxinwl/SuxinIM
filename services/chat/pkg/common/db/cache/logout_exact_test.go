package cache

import (
	"context"
	"errors"
	"github.com/redis/go-redis/v9"
	"testing"
)

type exactDeleteHook struct {
	calls int
	fail  bool
}

func (*exactDeleteHook) DialHook(next redis.DialHook) redis.DialHook { return next }
func (*exactDeleteHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}
func (h *exactDeleteHook) ProcessHook(_ redis.ProcessHook) redis.ProcessHook {
	return func(_ context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) != 3 || args[0] != "hdel" || args[1] != chatToken+"alice" || args[2] != "old-chat" {
			return errors.New("operation expanded beyond one registry field")
		}
		h.calls++
		if h.fail {
			return errors.New("injected Redis outage")
		}
		return nil
	}
}
func TestExactTokenDeletionUsesOnlyAtomicHDELAndPropagatesFailure(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"})
	defer client.Close()
	hook := &exactDeleteHook{}
	client.AddHook(hook)
	cache := NewTokenInterface(client, nil)
	if err := cache.DeleteTokenValue(context.Background(), "alice", "old-chat"); err != nil {
		t.Fatal(err)
	}
	hook.fail = true
	if err := cache.DeleteTokenValue(context.Background(), "alice", "old-chat"); err == nil {
		t.Fatal("Redis failure swallowed")
	}
	if hook.calls != 2 {
		t.Fatal("unexpected Redis operations")
	}
}
